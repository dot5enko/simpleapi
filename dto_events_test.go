package simpleapi

import (
	"testing"
)

func TestBeforeUpdateEvent(t *testing.T) {
	// OnBeforeUpdateCbAware

	event := MockEvent{}

	anyEvent := any(event)

	cbAware_, ok := anyEvent.(OnBeforeUpdateCbAware[MockAppContext])

	fakeContext := NewAppContext[MockAppContext](&MockAppContext{})

	if !ok {
		t.Log("type is not a beforeUpdate interface")
	} else {
		t.Log("invoking before update event")
		cbAware_.BeforeUpdate(fakeContext)
	}
}
