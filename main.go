package main

import (
	"flag"
	"github.com/gitchs/dagster_webserver_exporter/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"sync"
	"time"
)

var wg sync.WaitGroup
var configFile string
var config *utils.Config

func worker(instances chan *utils.WebServerInstance) {
	for instance := range instances {
		instance.Check()
		wg.Done()
	}
}

func init() {
	flag.StringVar(&configFile, "c", "./config.yaml", "yaml config filepath")
	flag.Parse()
	var errLoadConfig error
	if config, errLoadConfig = utils.LoadConfig(configFile); errLoadConfig != nil {
		log.Fatalf(`failed to load config file %s, error %s`, configFile, errLoadConfig.Error())
	}
}

func updateMetrics() {
	var numInstances2Monitor = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "dagster",
		Subsystem: "webserver",
		Name:      "num_instance_to_monitor",
		Help:      "total dagster instance to monitor",
	})
	numInstances2Monitor.Add(float64(len(config.Instances)))

	if len(config.Instances) > 0 {
		config.Instances.Init()
		checkQueue := make(chan *utils.WebServerInstance, 16)
		for i := 0; i < 4; i++ {
			go worker(checkQueue)
		}
		for {
			start := time.Now()
			wg.Add(len(config.Instances))
			for _, instance := range config.Instances {
				checkQueue <- &instance
			}
			wg.Wait()
			end := time.Now()
			duration := end.Sub(start)
			sleepStep := time.Second*time.Duration(config.Interval) - duration
			log.Printf(`take %s to check all instances, sleep %s`, duration.String(), sleepStep.String())
			time.Sleep(sleepStep)
		}
	}
}

func main() {
	go updateMetrics()
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
