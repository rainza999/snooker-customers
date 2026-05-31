package realtime

import (
	"testing"
	"time"
)

func TestPublishPOSOnlyNotifiesMatchingDivision(t *testing.T) {
	divisionOne, unsubscribeOne := SubscribePOS(1)
	defer unsubscribeOne()
	divisionTwo, unsubscribeTwo := SubscribePOS(2)
	defer unsubscribeTwo()

	PublishPOS(1, "table-opened")

	select {
	case event := <-divisionOne:
		if event.Action != "table-opened" {
			t.Fatalf("unexpected action: %s", event.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("matching division did not receive event")
	}

	select {
	case event := <-divisionTwo:
		t.Fatalf("other division received event: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}
