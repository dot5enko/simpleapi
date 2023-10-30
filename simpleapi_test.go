package simpleapi

import (
	"encoding/json"
	"testing"
)

func testToDto(t *testing.T) {

	event := MockEvent{
		Id:          100,
		SoftDeleted: false,
		Label:       "my first test event",
	}

	fields := GetFieldTags[MockAppContext, MockEvent](event)
	tName := GetObjectType(event)

	appCtx := AppContext[MockAppContext]{
		Data: &MockAppContext{},
	}

	appCtx.SetObjectsMapping(map[string]FieldsMapping{
		tName: fields,
	})

	reqAccess := RequestData{
		IsAdmin: false,
	}

	result := ToDto[MockEvent, MockAppContext](event, &appCtx, reqAccess)

	dto := result.Unwrap()

	jDto, _ := json.MarshalIndent(dto, "", "    ")

	t.Logf(" dto : %s", jDto)
}

// func testWriteProtectedField(t *testing.T) {

// 	user := types.User{
// 		Id:         100,
// 		CreatedAt:  time.Now(),
// 		RoleGroup:  0,
// 		Username:   "test",
// 		Secret:     "some secret",
// 		Balance:    500,
// 		SupabaseId: "sadwd",
// 		Email:      "test@test.com",
// 		Banned:     false,
// 		BanReason:  "",
// 		BanDue:     time.Time{},
// 	}

// 	fields := GetFieldTags[context.BetsAppContext, types.User](user)
// 	tName := GetObjectType(user)

// 	appCtx := AppContext[context.BetsAppContext]{
// 		Data:    &context.BetsAppContext{},
// 		Request: &gin.Context{},
// 	}

// 	appCtx.SetObjectsMapping(map[string]FieldsMapping{
// 		tName: fields,
// 	})

// 	reqAccess := RequestData{
// 		IsAdmin: false,
// 	}

// 	result := ToDto[types.User, context.BetsAppContext](user, &appCtx, reqAccess)

// 	dto := result.Unwrap()

// 	jDto, _ := json.MarshalIndent(dto, "", "    ")

// 	var newUser types.User

// 	fakeInput := gjson.Parse(`{"role_group": 5}`)

// 	writeSuperAdminReq := RequestData{
// 		IsAdmin:          true,
// 		RoleGroup:        2,
// 		AuthorizedUserId: 1,
// 	}

// 	err := appCtx.FillEntityFromDto(&newUser, fakeInput, nil, writeSuperAdminReq)

// 	if err != nil {
// 		t.Errorf("err: %s", err.Error())
// 	} else {
// 		if newUser.RoleGroup == 5 {
// 			t.Errorf("field were not protected")
// 		}
// 	}

// 	{
// 		var newUser types.User

// 		fakeInput := gjson.Parse(`{"role_group": 5}`)

// 		writeSuperAdminReq := RequestData{
// 			IsAdmin:          true,
// 			RoleGroup:        1,
// 			AuthorizedUserId: 1,
// 		}

// 		err := appCtx.FillEntityFromDto(&newUser, fakeInput, nil, writeSuperAdminReq)

// 		if err != nil {
// 			t.Errorf("err: %s", err.Error())
// 		} else {
// 			if newUser.RoleGroup != 5 {
// 				t.Errorf("field too much protected")
// 			}
// 		}

// 	}

// 	t.Logf(" dto user : %s", jDto)

// }

func TestSimpleapi(t *testing.T) {

	testToDto(t)
	// testWriteProtectedField(t)
}
