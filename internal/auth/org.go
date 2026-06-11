package auth

import (
	"fmt"
	"sort"
	"strings"
)

// OrgUnit is a group, department, or location (building/site) an admin manages.
// One type covers all three so the CRUD path is shared.
type OrgUnit struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // group, department, location
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"` // description / address
}

// ValidKind reports whether k is a known org-unit kind.
func ValidKind(k string) bool {
	switch k {
	case "group", "department", "location":
		return true
	}
	return false
}

// CreateOrgUnit adds a group / department / location.
func (s *Store) CreateOrgUnit(kind, name, detail string) (*OrgUnit, error) {
	if !ValidKind(kind) {
		return nil, fmt.Errorf("invalid kind")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orgUnits {
		if o.Kind == kind && strings.EqualFold(o.Name, name) {
			return nil, fmt.Errorf("a %s named %q already exists", kind, name)
		}
	}
	o := &OrgUnit{ID: newID(), Kind: kind, Name: name, Detail: strings.TrimSpace(detail)}
	s.orgUnits[o.ID] = o
	return o, s.persist()
}

// ListOrgUnits returns units of a kind, sorted by name.
func (s *Store) ListOrgUnits(kind string) []*OrgUnit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*OrgUnit
	for _, o := range s.orgUnits {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// OrgUnitName returns a unit's name, or "" if not found.
func (s *Store) OrgUnitName(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if o, ok := s.orgUnits[id]; ok {
		return o.Name
	}
	return ""
}

// CountOrgUnits returns how many units of a kind exist.
func (s *Store) CountOrgUnits(kind string) int { return len(s.ListOrgUnits(kind)) }

// DeleteOrgUnit removes a unit and clears it from any users.
func (s *Store) DeleteOrgUnit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.orgUnits, id)
	for _, u := range s.users {
		if u.DepartmentID == id {
			u.DepartmentID = ""
		}
		if u.LocationID == id {
			u.LocationID = ""
		}
		u.GroupIDs = removeStr(u.GroupIDs, id)
	}
	return s.persist()
}

// SetUserOrg assigns a user's department, location, and group membership.
func (s *Store) SetUserOrg(userID, deptID, locID string, groupIDs []string) error {
	return s.update(userID, func(u *User) {
		u.DepartmentID = deptID
		u.LocationID = locID
		u.GroupIDs = groupIDs
	})
}

func removeStr(ss []string, x string) []string {
	var out []string
	for _, s := range ss {
		if s != x {
			out = append(out, s)
		}
	}
	return out
}
