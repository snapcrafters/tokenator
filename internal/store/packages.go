package store

// Package is a generic representation of a package in a Canonical store.
// This could be a snap, a charm, a rock, etc.
type Package struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NewSnapPackage constructs and returns a new package of type "snap".
func NewSnapPackage(name string) Package {
	return Package{
		Name: name,
		Type: "snap",
	}
}
