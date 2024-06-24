package utils

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var queryBodyString string

func init() {
	const query = `query {
  workspaceOrError {
    __typename ... on Workspace {
      locationEntries {
        id
        name
        locationOrLoadError {
          __typename ... on PythonError {
            message
          }
          __typename ... on RepositoryLocation {
            isReloadSupported
          }
        }
      }
    }
  }
}`
	data := make(map[string]string)
	data["query"] = query
	rawData, _ := json.Marshal(data)
	queryBodyString = string(rawData)
	log.Printf("queryBodyString = %s", queryBodyString)
}

type BasicAuth struct {
	Username string
	Password string
}

type WebServerInstance struct {
	Name string
	URL  string
	Auth *BasicAuth
}

var instanceCheckTimes *prometheus.CounterVec
var instanceErrorTimes *prometheus.CounterVec
var instanceNumCodeLocations *prometheus.GaugeVec
var instanceCodeLocationLoadSuccess *prometheus.GaugeVec

func (i *WebServerInstance) Check() {
	labels := make(prometheus.Labels)
	labels["instance"] = i.Name
	instanceCheckTimes.WithLabelValues(i.Name).Inc()

	req, errNewRequest := http.NewRequest(http.MethodPost, i.URL, strings.NewReader(queryBodyString))
	if errNewRequest != nil {
		instanceErrorTimes.With(labels).Inc()
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if i.Auth != nil {
		req.SetBasicAuth(i.Auth.Username, i.Auth.Password)
	}
	res, errReq := http.DefaultClient.Do(req)
	if errReq != nil {
		log.Printf("instance [%s] send graphql failed, error %s", i.Name, errReq)
		instanceErrorTimes.With(labels).Inc()
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("instance [%s] request failed, http status %d", i.Name, res.StatusCode)
		instanceErrorTimes.With(labels).Inc()
		return
	}
	raw, errIO := io.ReadAll(res.Body)
	if errIO != nil {
		log.Printf("instance [%s] fetch response failed, error %s", i.Name, errIO)
		instanceErrorTimes.With(labels).Inc()
		return
	}

	var payload CodeLocationPayload
	errUnmarshal := json.Unmarshal(raw, &payload)
	if errUnmarshal != nil {
		log.Printf("instance [%s] unmarshal response failed, error %s", i.Name, errUnmarshal)
		instanceErrorTimes.With(labels).Inc()
		return
	}
	locations := payload.GetCodeLocation()
	instanceNumCodeLocations.With(labels).Set(float64(len(locations)))
	if locations == nil || len(locations) == 0 {
		log.Printf("instance [%s] has 0 code locations, something goes wrong", i.Name)
		instanceErrorTimes.With(labels).Inc()
		return
	}

	for _, location := range locations {
		labels2 := make(prometheus.Labels)
		labels2["instance"] = i.Name
		labels2["location"] = location.Name
		log.Printf("instance %s, location %s, error %v", i.Name, location.Name, location.HasError())
		if location.HasError() {
			instanceCodeLocationLoadSuccess.With(labels2).Set(float64(0))
		} else {
			instanceCodeLocationLoadSuccess.With(labels2).Set(float64(1))
		}
	}

}

type WebServerInstances []WebServerInstance

func (is *WebServerInstances) Init() {
	labels := []string{"instance"}
	instanceCheckTimes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dagster",
		Subsystem: "webserver",
		Name:      "instance_check_times",
	}, labels)

	instanceErrorTimes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dagster",
		Subsystem: "webserver",
		Name:      "instance_http_error_times",
	}, labels)

	instanceCodeLocationLoadSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dagster",
		Subsystem: "webserver",
		Name:      "instance_code_location_load_success",
	}, []string{"instance", "location"})

	instanceNumCodeLocations = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dagster",
		Subsystem: "webserver",
		Name:      "instance_num_code_locations",
	}, []string{"instance"})
}

type Config struct {
	Interval  int
	Instances WebServerInstances
}

func LoadConfig(filename string) (*Config, error) {
	fd, errIO := os.Open(filename)
	if errIO != nil {
		return nil, errIO
	}
	defer fd.Close()
	raw, errIO := io.ReadAll(fd)
	if errIO != nil {
		return nil, errIO
	}
	config := new(Config)
	err := yaml.Unmarshal(raw, config)
	if err != nil {
		config = nil
	}
	if config.Interval <= 0 {
		config.Interval = 30
	}
	return config, err
}
