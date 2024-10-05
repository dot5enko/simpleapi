package simpleapi

import "sort"

type UserOwnershipData struct {
	ResourceOwnedId uint64
	ResourceType    string
}

type UserDataExtended struct {
	Resources []UserOwnershipData
}

type UserRole struct {
	Name              string // unique
	Priority          int    // higher - more rights
	GetResourcesOwned func(req RequestData) UserDataExtended
}

var userRolesMap = map[string]UserRole{}
var userRolesSorted = []UserRole{}

func RegisterUserRole(role UserRole) {
	userRolesMap[role.Name] = role

	nonSorted := []UserRole{}

	for _, it := range userRolesMap {
		nonSorted = append(nonSorted, it)
	}

	sort.Slice(nonSorted, func(i, j int) bool {
		return nonSorted[i].Priority > nonSorted[j].Priority
	})

	userRolesSorted = nonSorted
}
