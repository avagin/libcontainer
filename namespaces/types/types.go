package types

import "errors"

type (
	Namespace struct {
		Key   string `json:"key,omitempty"`
		Value int    `json:"value,omitempty"`
		File  string `json:"file,omitempty"`
	}
	Namespaces []*Namespace
)

// namespaceList is used to convert the libcontainer types
// into the names of the files located in /proc/<pid>/ns/* for
// each namespace
var (
	namespaceList      = Namespaces{}
	ErrUnkownNamespace = errors.New("Unknown namespace")
	ErrUnsupported     = errors.New("Unsupported method")
)

func (ns *Namespace) String() string {
	return ns.Key
}

func GetNamespace(key string) *Namespace {
	for _, ns := range namespaceList {
		if ns.Key == key {
			cpy := *ns
			return &cpy
		}
	}
	return nil
}

// Contains returns true if the specified Namespace is
// in the slice
func (n Namespaces) Contains(ns string) bool {
	return n.Get(ns) != nil
}

func (n Namespaces) Get(ns string) *Namespace {
	for _, nsp := range n {
		if nsp != nil && nsp.Key == ns {
			return nsp
		}
	}
	return nil
}

// GetNamespaceFlags parses the container's Namespaces options to set the correct
// flags on clone, unshare, and setns
func GetNamespaceFlags(namespaces map[string]bool) (flag int) {
	for key, enabled := range namespaces {
		if enabled {
			if ns := GetNamespace(key); ns != nil {
				flag |= ns.Value
			}
		}
	}
	return flag
}
