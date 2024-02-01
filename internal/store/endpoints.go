package store

// StoreEndpoints represents the the base and storage URLs for a given environment/store.
type StoreEndpoints struct {
	BaseURL    string
	StorageURL string
}

// SNAP_STORE_ENDPOINTS represents the set of endpoints used when interacting with
// the production Snap store.
var SNAP_STORE_ENDPOINTS = StoreEndpoints{
	BaseURL:    "https://dashboard.snapcraft.io",
	StorageURL: "https://upload.apps.staging.ubuntu.com",
}

// StoreAuthEndpoints represents the authn/authz endpoints to use for a given environment/store.
type StoreAuthEndpoints struct {
	AuthURL           string
	Namespace         string
	Whoami            string
	Tokens            string
	TokensExchange    string
	TokensRefresh     string
	ValidPackageTypes []string
}

// UBUNTU_ONE_SNAP_STORE_AUTH_ENDPOINTS represents the set of endpoints used to authenticate
// against the Snap Store using Ubuntu One credentials.
var UBUNTU_ONE_SNAP_STORE_AUTH_ENDPOINTS = StoreAuthEndpoints{
	AuthURL:           "https://login.ubuntu.com",
	Namespace:         "snap",
	Whoami:            "/api/v2/tokens/whoami",
	Tokens:            "/dev/api/acl/",
	TokensExchange:    "/api/v2/tokens/discharge",
	TokensRefresh:     "/api/v2/tokens/refresh",
	ValidPackageTypes: []string{"snap"},
}
