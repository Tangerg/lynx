package httpreq

import (
	"net/http"
	"strings"
)

// Method is an HTTP method exposed by the tool contract.
type Method string

const (
	MethodGET    Method = http.MethodGet
	MethodHEAD   Method = http.MethodHead
	MethodPOST   Method = http.MethodPost
	MethodPUT    Method = http.MethodPut
	MethodPATCH  Method = http.MethodPatch
	MethodDELETE Method = http.MethodDelete
)

// Normalize applies the wire default and canonical HTTP casing.
func (m Method) Normalize() Method {
	normalized := Method(strings.ToUpper(strings.TrimSpace(string(m))))
	if normalized == "" {
		return MethodGET
	}
	return normalized
}

func (m Method) Validate() error {
	switch m.Normalize() {
	case MethodGET, MethodHEAD, MethodPOST, MethodPUT, MethodPATCH, MethodDELETE:
		return nil
	default:
		return ErrInvalidMethod
	}
}
