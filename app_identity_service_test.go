package centraldogma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestCreateAppIdentity(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	request := &CreateAppIdentityRequest{
		AppID:         "my-app",
		Type:          AppIdentityTypeToken,
		IsSystemAdmin: false,
	}

	mux.HandleFunc("/api/v1/appIdentities", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		testAuthorization(t, r)
		testURLQuery(t, r, "appId", "my-app")
		testURLQuery(t, r, "type", "TOKEN")
		testURLQuery(t, r, "isSystemAdmin", "false")
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Location", "/api/v1/appIdentities/my-app")
		fmt.Fprint(w, `{"appId":"my-app","type":"TOKEN","secret":"appToken-secret","systemAdmin":false}`)
	})

	appIdentity, httpStatusCode, _ := c.CreateAppIdentity(context.Background(), request)
	testStatusCode(t, httpStatusCode, 201)

	want := &AppIdentity{AppID: "my-app", Type: AppIdentityTypeToken, Secret: "appToken-secret"}
	if !reflect.DeepEqual(appIdentity, want) {
		t.Errorf("CreateAppIdentity returned %+v, want %+v", appIdentity, want)
	}
}

func TestListAppIdentities(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/api/v1/appIdentities", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		fmt.Fprint(w, `[{"appId":"app1","type":"TOKEN"},{"appId":"app2","type":"CERTIFICATE","certificateId":"cert-2"}]`)
	})

	appIDs, httpStatusCode, _ := c.ListAppIdentities(context.Background())
	testStatusCode(t, httpStatusCode, 200)

	want := []*AppIdentity{
		{AppID: "app1", Type: AppIdentityTypeToken},
		{AppID: "app2", Type: AppIdentityTypeCertificate, CertificateID: "cert-2"},
	}
	if !reflect.DeepEqual(appIDs, want) {
		t.Errorf("ListAppIdentities returned %+v, want %+v", appIDs, want)
	}
}

func TestUpdateAppIdentityStatus(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/api/v1/appIdentities/my-app", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPatch)
		testHeader(t, r, "Content-Type", "application/json")
		testBody(t, r, `{"status":"inactive"}`+"\n")
		fmt.Fprint(w, `{"appId":"my-app","type":"TOKEN","deactivation":{"user":"u","timestamp":"2026-01-01T00:00:00Z"}}`)
	})

	appID, httpStatusCode, _ := c.UpdateAppIdentityStatus(context.Background(), "my-app", AppIdentityStatusInactive)
	testStatusCode(t, httpStatusCode, 200)

	if appID.AppID != "my-app" {
		t.Errorf("UpdateAppIdentityStatus returned %+v, want appId my-app", appID)
	}
}

func TestUpdateAppIdentityLevel(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/api/v1/appIdentities/my-app/level", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPatch)
		testHeader(t, r, "Content-Type", "application/json")
		testBody(t, r, `{"level":"SYSTEMADMIN"}`+"\n")
		fmt.Fprint(w, `{"appId":"my-app","type":"TOKEN","systemAdmin":true}`)
	})

	appID, httpStatusCode, _ := c.UpdateAppIdentityLevel(context.Background(), "my-app", AppIdentityLevelSystemAdmin)
	testStatusCode(t, httpStatusCode, 200)

	want := &AppIdentity{AppID: "my-app", Type: AppIdentityTypeToken, SystemAdmin: true}
	if !reflect.DeepEqual(appID, want) {
		t.Errorf("UpdateAppIdentityLevel returned %+v, want %+v", appID, want)
	}
}

func TestProjectRoleAPIs(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	input := map[string]string{"id": "my-app", "role": "MEMBER"}
	mux.HandleFunc("/api/v1/metadata/foo/appIdentities", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body, input) {
			t.Errorf("Request body = %+v, want %+v", body, input)
		}
		fmt.Fprint(w, `49`)
	})
	mux.HandleFunc("/api/v1/metadata/foo/appIdentities/my-app", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			testHeader(t, r, "Content-Type", "application/json-patch+json")
			testBody(t, r, `[{"op":"replace", "path":"/role", "value":"OWNER"}]`)
			fmt.Fprint(w, `50`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	})

	revision, httpStatusCode, _ := c.AddAppIdentityToProject(context.Background(), "foo", "my-app", ProjectRoleMember)
	testStatusCode(t, httpStatusCode, 200)
	if revision != 49 {
		t.Errorf("AddAppIdentityToProject returned %v, want %v", revision, 49)
	}

	revision, httpStatusCode, _ = c.UpdateAppIdentityProjectRole(context.Background(), "foo", "my-app", ProjectRoleOwner)
	testStatusCode(t, httpStatusCode, 200)
	if revision != 50 {
		t.Errorf("UpdateAppIdentityProjectRole returned %v, want %v", revision, 50)
	}

	revision, httpStatusCode, _ = c.RemoveAppIdentityFromProject(context.Background(), "foo", "my-app")
	testStatusCode(t, httpStatusCode, 204)
	if revision != 0 {
		t.Errorf("RemoveAppIdentityFromProject returned %v, want %v", revision, 0)
	}
}

func TestRepositoryRoleAPIs(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	input := map[string]string{"id": "my-app", "role": "WRITE"}
	mux.HandleFunc("/api/v1/metadata/foo/repos/bar/roles/appIdentities", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body, input) {
			t.Errorf("Request body = %+v, want %+v", body, input)
		}
		fmt.Fprint(w, `51`)
	})
	mux.HandleFunc("/api/v1/metadata/foo/repos/bar/roles/appIdentities/my-app", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodDelete)
		w.WriteHeader(http.StatusNoContent)
	})

	revision, httpStatusCode, _ := c.AddAppIdentityToRepository(context.Background(), "foo", "bar", "my-app", RepositoryRoleWrite)
	testStatusCode(t, httpStatusCode, 200)
	if revision != 51 {
		t.Errorf("AddAppIdentityToRepository returned %v, want %v", revision, 51)
	}

	revision, httpStatusCode, _ = c.RemoveAppIdentityFromRepository(context.Background(), "foo", "bar", "my-app")
	testStatusCode(t, httpStatusCode, 204)
	if revision != 0 {
		t.Errorf("RemoveAppIdentityFromRepository returned %v, want %v", revision, 0)
	}
}

func TestAppIdentityRevisionUnmarshalJSON(t *testing.T) {
	var fromNumber appIdentityRevision
	if err := json.Unmarshal([]byte(`49`), &fromNumber); err != nil {
		t.Fatalf("json.Unmarshal(number) error = %v", err)
	}
	if int(fromNumber) != 49 {
		t.Fatalf("fromNumber = %d, want %d", fromNumber, 49)
	}

	var fromObject appIdentityRevision
	if err := json.Unmarshal([]byte(`{"revision":50}`), &fromObject); err != nil {
		t.Fatalf("json.Unmarshal(object) error = %v", err)
	}
	if int(fromObject) != 50 {
		t.Fatalf("fromObject = %d, want %d", fromObject, 50)
	}

	var invalid appIdentityRevision
	if err := json.Unmarshal([]byte(`{"rev":51}`), &invalid); err == nil {
		t.Fatal("json.Unmarshal(invalid) expected error, got nil")
	}
}
