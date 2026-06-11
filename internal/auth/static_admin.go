package auth

import (
	"html/template"
	"os"
	"path/filepath"
)

// staticAdminNav links between the static admin pages (relative .html files).
const staticAdminNav = `<div class="bar"><div class="logo" style="margin:0">Look<b>out</b> &middot; Admin</div>
  <div class="row" style="gap:1.1rem;font-size:.9rem;flex-wrap:wrap"><a href="users.html">Users</a><a href="groups.html">Groups</a><a href="departments.html">Departments</a><a href="locations.html">Locations</a><a href="index.html">&larr; Dashboard</a></div></div>`

// WriteStaticAdmin renders the admin console (users / groups / departments /
// locations) into dir with sample data, so the static demo shows it. Forms are
// present but inert in the static copy.
func WriteStaticAdmin(dir string) error {
	nav := template.HTML(staticAdminNav)
	me := &User{ID: "me", Email: "admin@lookout.app", Role: RoleOwner}
	depts := []*OrgUnit{
		{ID: "d1", Kind: "department", Name: "Engineering", Detail: "Software & platform"},
		{ID: "d2", Kind: "department", Name: "IT Operations", Detail: "Infrastructure"},
	}
	locs := []*OrgUnit{
		{ID: "l1", Kind: "location", Name: "HQ — Building A", Detail: "123 Main St, Seattle"},
		{ID: "l2", Kind: "location", Name: "Data Center West", Detail: "Reno, NV"},
	}
	groups := []*OrgUnit{
		{ID: "g1", Kind: "group", Name: "On-call", Detail: "Pager rotation"},
		{ID: "g2", Kind: "group", Name: "Admins", Detail: "Full access"},
	}
	users := []*User{
		{ID: "u1", Email: "admin@lookout.app", Name: "Avery Admin", Role: RoleOwner, MFAEnabled: true, DepartmentID: "d2", LocationID: "l1", GroupIDs: []string{"g2"}},
		{ID: "u2", Email: "sam@lookout.app", Name: "Sam Operator", Role: RoleOperator, DepartmentID: "d1", LocationID: "l1", GroupIDs: []string{"g1"}},
		{ID: "u3", Email: "val@lookout.app", Name: "Val Viewer", Role: RoleViewer},
	}
	roles := []Role{RoleOwner, RoleAdmin, RoleOperator, RoleViewer}

	if err := writeAuthFile(filepath.Join(dir, "users.html"), usersTmpl, map[string]any{
		"Nav": nav, "Me": me, "Users": users, "Roles": roles,
		"Departments": depts, "Locations": locs, "Groups": groups,
	}); err != nil {
		return err
	}
	for kind, units := range map[string][]*OrgUnit{"group": groups, "department": depts, "location": locs} {
		file := map[string]string{"group": "groups.html", "department": "departments.html", "location": "locations.html"}[kind]
		if err := writeAuthFile(filepath.Join(dir, file), orgTmpl, map[string]any{
			"Nav": nav, "Kind": kind, "Title": kindTitle(kind), "DetailLabel": kindDetailLabel(kind), "Units": units,
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeAuthFile(path string, t *template.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}
