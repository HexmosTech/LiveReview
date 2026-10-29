package scim

import "strconv"

const (
	SchemaUser         = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaListResponse = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaPatchOp      = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaError        = "urn:ietf:params:scim:api:messages:2.0:Error"
)

// UserResource is the minimal SCIM User representation LiveReview speaks -
// enough for Okta/Azure AD/Google's lifecycle-sync use case (create,
// look up by userName/email, toggle active). Fields no customer IdP relies
// on for this use case (groups, x509Certificates, etc.) are intentionally
// omitted rather than stubbed out.
type UserResource struct {
	Schemas  []string        `json:"schemas"`
	ID       string          `json:"id"`
	UserName string          `json:"userName"`
	Name     *NameResource   `json:"name,omitempty"`
	Emails   []EmailResource `json:"emails,omitempty"`
	Active   bool            `json:"active"`
	Meta     *MetaResource   `json:"meta,omitempty"`
}

type NameResource struct {
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type EmailResource struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary,omitempty"`
}

type MetaResource struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

type ListResponse struct {
	Schemas      []string       `json:"schemas"`
	TotalResults int            `json:"totalResults"`
	ItemsPerPage int            `json:"itemsPerPage"`
	StartIndex   int            `json:"startIndex"`
	Resources    []UserResource `json:"Resources"`
}

// PatchRequest is the body of a SCIM PATCH /Users/:id call. IdPs send this
// to deprovision on offboarding: {"Operations":[{"op":"replace","value":{"active":false}}]}.
type PatchRequest struct {
	Schemas    []string         `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}

type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}

// ErrorResponse is the SCIM-shaped error body every handler returns on
// failure, per RFC 7644 §3.12.
type ErrorResponse struct {
	Schemas []string `json:"schemas"`
	Status  string   `json:"status"`
	Detail  string   `json:"detail"`
}

func NewError(status int, detail string) ErrorResponse {
	return ErrorResponse{
		Schemas: []string{SchemaError},
		Status:  strconv.Itoa(status),
		Detail:  detail,
	}
}
