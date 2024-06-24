package utils

const (
	TypeLocation = "RepositoryLocation"
	TypeError    = "PythonError"
)

type CodeLocation struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	LocationOrLoadError struct {
		Typename          string `json:"__typename"`
		Message           string `json:"message"`
		IsReloadSupported bool   `json:"isReloadSupported"`
	} `json:"locationOrLoadError"`
}

func (l *CodeLocation) HasError() bool {
	return l.LocationOrLoadError.Typename == TypeError
}

type CodeLocationPayload struct {
	Data struct {
		WorkspaceOrError struct {
			Typename        string         `json:"__typename"`
			LocationEntries []CodeLocation `json:"locationEntries"`
		} `json:"workspaceOrError"`
	} `json:"data"`
}

func (p *CodeLocationPayload) GetCodeLocation() []CodeLocation {
	return p.Data.WorkspaceOrError.LocationEntries
}
