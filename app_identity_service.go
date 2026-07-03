package centraldogma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
)

type appIdentityService service

// AppIdentityType represents a type of app identity.
type AppIdentityType string

const (
	AppIdentityTypeToken       AppIdentityType = "TOKEN"
	AppIdentityTypeCertificate AppIdentityType = "CERTIFICATE"
)

// ProjectRole represents a role of an app identity in a project.
type ProjectRole string

const (
	ProjectRoleOwner  ProjectRole = "OWNER"
	ProjectRoleMember ProjectRole = "MEMBER"
)

// RepositoryRole represents a role of an app identity in a repository.
type RepositoryRole string

const (
	RepositoryRoleAdmin RepositoryRole = "ADMIN"
	RepositoryRoleWrite RepositoryRole = "WRITE"
	RepositoryRoleRead  RepositoryRole = "READ"
)

// AppIdentityStatus represents activation status of an app identity.
type AppIdentityStatus string

const (
	AppIdentityStatusActive   AppIdentityStatus = "active"
	AppIdentityStatusInactive AppIdentityStatus = "inactive"
)

// AppIdentityLevel represents permission level of an app identity.
type AppIdentityLevel string

const (
	AppIdentityLevelUser        AppIdentityLevel = "USER"
	AppIdentityLevelSystemAdmin AppIdentityLevel = "SYSTEMADMIN"
)

// UserAndTimestamp represents who performed an action and when.
type UserAndTimestamp struct {
	User      string `json:"user,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// AppIdentity represents an app identity in the Central Dogma server.
type AppIdentity struct {
	AppID            string            `json:"appId"`
	Type             AppIdentityType   `json:"type,omitempty"`
	Secret           string            `json:"secret,omitempty"`
	CertificateID    string            `json:"certificateId,omitempty"`
	SystemAdmin      bool              `json:"systemAdmin,omitempty"`
	AllowGuestAccess bool              `json:"allowGuestAccess,omitempty"`
	Creation         *UserAndTimestamp `json:"creation,omitempty"`
	Deactivation     *UserAndTimestamp `json:"deactivation,omitempty"`
	Deletion         *UserAndTimestamp `json:"deletion,omitempty"`
}

// CreateAppIdentityRequest is used for creating an app identity.
type CreateAppIdentityRequest struct {
	AppID         string
	Type          AppIdentityType
	IsSystemAdmin bool
	Secret        string
	CertificateID string
}

type appIdentityRevision int

func (r *appIdentityRevision) UnmarshalJSON(data []byte) error {
	// Some endpoints return a plain number (e.g. 49) while others may return {"revision":49}.
	var value int
	if err := json.Unmarshal(data, &value); err == nil {
		*r = appIdentityRevision(value)
		return nil
	}

	var payload struct {
		Revision *int `json:"revision"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		if payload.Revision != nil {
			*r = appIdentityRevision(*payload.Revision)
			return nil
		}
	}

	return fmt.Errorf("unsupported revision payload: %s", string(data))
}

func (a *appIdentityService) create(ctx context.Context, request *CreateAppIdentityRequest) (*AppIdentity, int, error) {
	u, err := url.Parse(path.Join(defaultPathPrefix, appIdentities))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	q := u.Query()
	q.Set("appId", request.AppID)
	q.Set("type", string(request.Type))
	q.Set("isSystemAdmin", strconv.FormatBool(request.IsSystemAdmin))
	if request.Secret != "" {
		q.Set("secret", request.Secret)
	}
	if request.CertificateID != "" {
		q.Set("certificateId", request.CertificateID)
	}
	u.RawQuery = q.Encode()

	req, err := a.client.newRequest(http.MethodPost, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	appIdentity := new(AppIdentity)
	httpStatusCode, err := a.client.do(ctx, req, appIdentity, false)
	if err != nil {
		return nil, httpStatusCode, err
	}

	return appIdentity, httpStatusCode, nil
}

func (a *appIdentityService) list(ctx context.Context) ([]*AppIdentity, int, error) {
	u, err := url.Parse(path.Join(defaultPathPrefix, appIdentities))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	req, err := a.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var appIDs []*AppIdentity
	httpStatusCode, err := a.client.do(ctx, req, &appIDs, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return appIDs, httpStatusCode, nil
}

func (a *appIdentityService) remove(ctx context.Context, appID string) (*AppIdentity, int, error) {
	u, err := url.Parse(path.Join(defaultPathPrefix, appIdentities, appID))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	req, err := a.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	appIdentity := new(AppIdentity)
	httpStatusCode, err := a.client.do(ctx, req, appIdentity, false)
	if err != nil {
		return nil, httpStatusCode, err
	}

	return appIdentity, httpStatusCode, nil
}

func (a *appIdentityService) updateStatus(
	ctx context.Context, appID string, status AppIdentityStatus) (*AppIdentity, int, error) {
	u, err := url.Parse(path.Join(defaultPathPrefix, appIdentities, appID))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	body := map[string]string{"status": string(status)}
	req, err := a.client.newRequest(http.MethodPatch, u, body)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	// This endpoint accepts application/json, not JSON patch.
	req.Header.Set("Content-Type", "application/json")

	appIdentity := new(AppIdentity)
	httpStatusCode, err := a.client.do(ctx, req, appIdentity, false)
	if err != nil {
		return nil, httpStatusCode, err
	}

	return appIdentity, httpStatusCode, nil
}

func (a *appIdentityService) updateLevel(
	ctx context.Context, appID string, level AppIdentityLevel) (*AppIdentity, int, error) {
	u, err := url.Parse(path.Join(defaultPathPrefix, appIdentities, appID, "level"))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	body := map[string]string{"level": string(level)}
	req, err := a.client.newRequest(http.MethodPatch, u, body)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	// This endpoint accepts application/json, not JSON patch.
	req.Header.Set("Content-Type", "application/json")

	appIdentity := new(AppIdentity)
	httpStatusCode, err := a.client.do(ctx, req, appIdentity, false)
	if err != nil {
		return nil, httpStatusCode, err
	}

	return appIdentity, httpStatusCode, nil
}

func (a *appIdentityService) addToProject(
	ctx context.Context, projectName, appID string, role ProjectRole) (int, int, error) {
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		metadata, projectName,
		appIdentities,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	body := map[string]string{"id": appID, "role": string(role)}
	req, err := a.client.newRequest(http.MethodPost, u, body)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	var revision appIdentityRevision
	httpStatusCode, err := a.client.do(ctx, req, &revision, false)
	if err != nil {
		return -1, httpStatusCode, err
	}

	return int(revision), httpStatusCode, nil
}

func (a *appIdentityService) updateProjectRole(
	ctx context.Context, projectName, appID string, role ProjectRole) (int, int, error) {
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		metadata, projectName,
		appIdentities, appID,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	body := `[{"op":"replace", "path":"/role", "value":"` + string(role) + `"}]`
	req, err := a.client.newRequest(http.MethodPatch, u, body)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	var revision appIdentityRevision
	httpStatusCode, err := a.client.do(ctx, req, &revision, false)
	if err != nil {
		return -1, httpStatusCode, err
	}

	return int(revision), httpStatusCode, nil
}

func (a *appIdentityService) removeFromProject(ctx context.Context, projectName, appID string) (int, int, error) {
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		metadata, projectName,
		appIdentities, appID,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	req, err := a.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	var revision appIdentityRevision
	httpStatusCode, err := a.client.do(ctx, req, &revision, false)
	if err != nil {
		return -1, httpStatusCode, err
	}

	return int(revision), httpStatusCode, nil
}

func (a *appIdentityService) addToRepository(
	ctx context.Context, projectName, repoName, appID string, role RepositoryRole) (int, int, error) {
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		metadata, projectName,
		repos, repoName,
		"roles", appIdentities,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	body := map[string]string{"id": appID, "role": string(role)}
	req, err := a.client.newRequest(http.MethodPost, u, body)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	var revision appIdentityRevision
	httpStatusCode, err := a.client.do(ctx, req, &revision, false)
	if err != nil {
		return -1, httpStatusCode, err
	}

	return int(revision), httpStatusCode, nil
}

func (a *appIdentityService) removeFromRepository(
	ctx context.Context, projectName, repoName, appID string) (int, int, error) {
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		metadata, projectName,
		repos, repoName,
		"roles", appIdentities, appID,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	req, err := a.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	var revision appIdentityRevision
	httpStatusCode, err := a.client.do(ctx, req, &revision, false)
	if err != nil {
		return -1, httpStatusCode, err
	}

	return int(revision), httpStatusCode, nil
}
