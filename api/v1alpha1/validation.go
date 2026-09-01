package v1alpha1

import (
	"fmt"
	"regexp"
)

var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateModules checks module names for DNS-1123, uniqueness, and reserved ids.
func ValidateModules(modules []ModuleSpec) error {
	seen := make(map[string]struct{}, len(modules))
	for _, mod := range modules {
		if mod.Name == "" {
			continue
		}
		if mod.Name == ReservedModuleName {
			return fmt.Errorf("module name %q is reserved", ReservedModuleName)
		}
		if !dns1123Label.MatchString(mod.Name) {
			return fmt.Errorf("module name %q is not a DNS-1123 label", mod.Name)
		}
		if _, dup := seen[mod.Name]; dup {
			return fmt.Errorf("duplicate module name %q", mod.Name)
		}
		seen[mod.Name] = struct{}{}
	}
	return nil
}
