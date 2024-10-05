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
		GetOwnership: func(req RequestData) UserOwnershipData {
			return UserOwnershipData{
				ResourceOwnedId: 1,
			}
		},
	})

	RegisterUserRole(UserRole{
		Name:     "admin",
		Priority: 3,
	})

	RegisterUserRole(UserRole{
		Name:     "superadmin",
		Priority: 4,
	})

	t.Logf("first role : %s", userRolesSorted[0].Name)

}
