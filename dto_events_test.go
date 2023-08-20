package simpleapi

import (
	"testing"
)

func TestBeforeUpdateEvent(t *testing.T) {
	// OnBeforeUpdateCbAware

	event := MockEvent{}

	anyEvent := any(event)

	_, ok := anyEvent.(OnBeforeUpdateCbAware[MockAppContext])

	if !ok {
		t.Log("type is not a beforeUpdate interface")
	}
}
