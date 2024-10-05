package simpleapi

import "testing"

func TestRoles(t *testing.T) {

	RegisterUserRole(UserRole{
		Name:     "authorized",
		Priority: 1,
	})

	RegisterUserRole(UserRole{
		Name:     "aff2",
		Priority: 2,
	})

	RegisterUserRole(UserRole{
		Name:     "admin",
		Priority: 3,
	})

	RegisterUserRole(UserRole{
		Name:     "superadmin",
		Priority: 4,
	})

	if userRolesSorted[0].Name != "superadmin" {
		t.Logf("sorting of roles is not valid")
	}

}
