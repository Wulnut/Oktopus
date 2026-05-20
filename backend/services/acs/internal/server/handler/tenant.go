package handler

import "strings"

// ParseTenantSlug extracts the tenant slug from a CWMP request path.
// Examples (route="/acs"): "/acs" -> default, "/acs/sei/" -> sei, "/acs/sei" -> sei.
func ParseTenantSlug(path, route string) string {
	path = strings.TrimSuffix(path, "/")
	route = strings.TrimSuffix(route, "/")
	if route == "" {
		route = "/acs"
	}
	if path == route {
		return DEFAULT_TENANT
	}
	prefix := route + "/"
	if !strings.HasPrefix(path, prefix) {
		return DEFAULT_TENANT
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return DEFAULT_TENANT
	}
	tenant, _, _ := strings.Cut(rest, "/")
	if tenant == "" {
		return DEFAULT_TENANT
	}
	return tenant
}

// TenantFromCwmpAdapterSubject extracts the tenant from a
// cwmp-adapter.v1.<tenant>.<sn>.<action> subject. Returns "" when the subject
// does not match the expected 5-token layout.
func TenantFromCwmpAdapterSubject(subject string) string {
	const expectedPrefix = "cwmp-adapter.v1."
	if !strings.HasPrefix(subject, expectedPrefix) {
		return ""
	}
	parts := strings.Split(subject, ".")
	// cwmp-adapter . v1 . <tenant> . <sn> . <action>
	if len(parts) < 5 {
		return ""
	}
	return parts[2]
}

func cwmpInfoSubject(tenant, sn string) string {
	return NATS_CWMP_SUBJECT_PREFIX + tenant + "." + sn + ".info"
}

func cwmpStatusSubject(tenant, sn string) string {
	return NATS_CWMP_SUBJECT_PREFIX + tenant + "." + sn + ".status"
}

func cpeTenantSlug(cpe CPE) string {
	if cpe.TenantSlug != "" {
		return cpe.TenantSlug
	}
	return DEFAULT_TENANT
}
