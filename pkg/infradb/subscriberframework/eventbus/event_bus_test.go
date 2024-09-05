package eventbus

import (
	"log"
	"sync"
	"testing"
	"time"
)

type moduleCiHandler struct {
	receivedEvents []*ObjectData
	sync.Mutex
}

// Constants
const (
	TestEvent         = "testEvent"
	TestEventpriority = "testEventpriority"
	TestEventChBusy   = "testEventChBusy"
	TestEventUnsub    = "testEventUnsub"
)

func (h *moduleCiHandler) HandleEvent(eventType string, objectData *ObjectData) {
	h.Lock()
	defer h.Unlock()
	h.receivedEvents = append(h.receivedEvents, objectData)
	switch eventType {
	case TestEvent:
	case TestEventpriority:
	case TestEventChBusy:
	case TestEventUnsub:
		log.Printf("received event type %s", eventType)
	default:
		log.Panicf("error: Unknown event type %s", eventType)
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	handler := &moduleCiHandler{}

	EventBus := NewEventBus()
	EventBus.StartSubscriber("testModule", TestEvent, 1, handler)
	time.Sleep(10 * time.Millisecond)

	objectData := &ObjectData{
		ResourceVersion: "v1",
		Name:            "testObject",
		NotificationID:  "123",
	}

	subscribers := EventBus.GetSubscribers(TestEvent)
	if len(subscribers) == 0 {
		t.Errorf("No subscribers found for event type 'testEvent'")
	}
	subscriber := subscribers[0]

	err := EventBus.Publish(objectData, subscriber)
	if err != nil {
		t.Errorf("Publish() failed with error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	handler.Lock()
	if len(handler.receivedEvents) != 1 {
		t.Errorf("Event was not received by the handler as expected")
	}
	if handler.receivedEvents[0] != objectData {
		t.Errorf("Event data was not received by the handler as expected")
	}
	handler.Unlock()

	EventBus.Unsubscribe(subscriber)
}

func TestPriorityOrderWithStartSubscriber(t *testing.T) {
	handler1 := &moduleCiHandler{}
	handler2 := &moduleCiHandler{}

	EventBus := NewEventBus()

	EventBus.StartSubscriber("testModule1", TestEventpriority, 2, handler1)
	EventBus.StartSubscriber("testModule2", TestEventpriority, 1, handler2)

	time.Sleep(10 * time.Millisecond)

	subscribers := EventBus.GetSubscribers(TestEventpriority)
	if len(subscribers) != 2 {
		t.Errorf("Expected 2 subscribers, got %d", len(subscribers))
	}
	if subscribers[0].Priority > subscribers[1].Priority {
		t.Errorf("Subscribers are not sorted by priority")
	}

	for _, sub := range subscribers {
		EventBus.Unsubscribe(sub)
	}
}
func TestPublishChannelBusyWithStartSubscriber(t *testing.T) {
	handler := &moduleCiHandler{}
	EventBus := NewEventBus()
	EventBus.StartSubscriber("testModuleChBusy", TestEventChBusy, 1, handler)

	time.Sleep(10 * time.Millisecond)

	subscribers := EventBus.GetSubscribers(TestEventChBusy)
	if len(subscribers) == 0 {
		t.Errorf("No subscribers found for event type 'testEventChBusy'")
	}
	subscriber := subscribers[0]

	subscriber.Ch <- &ObjectData{}

	objectData := &ObjectData{
		ResourceVersion: "v1",
		Name:            "testObject",
		NotificationID:  "123",
	}
	err := EventBus.Publish(objectData, subscriber)
	if err == nil {
		t.Errorf("Expected an error when publishing to a busy channel, but got nil")
	}

	EventBus.Unsubscribe(subscriber)
}
func TestUnsubscribeEvent(t *testing.T) {
	handler := &moduleCiHandler{}
	EventBus := NewEventBus()
	EventBus.StartSubscriber("testModuleUnsub", TestEventUnsub, 1, handler)

	subscribers := EventBus.GetSubscribers(TestEventUnsub)
	if len(subscribers) == 0 {
		t.Errorf("No subscribers found for event type 'testEventUnsub'")
	}
	subscriber := subscribers[0]

	EventBus.UnsubscribeEvent(subscriber, TestEventUnsub)

	subscribers = EventBus.GetSubscribers(TestEventUnsub)
	for _, sub := range subscribers {
		if sub == subscriber {
			t.Errorf("Subscriber was not successfully unsubscribed")
		}
	}
}

func TestUnsubscribe(t *testing.T) {
	handler := &moduleCiHandler{}
	EventBus := NewEventBus()
	EventBus.StartSubscriber("testModuleUnsub", TestEventUnsub, 1, handler)

	subscribers := EventBus.GetSubscribers(TestEventUnsub)
	if len(subscribers) == 0 {
		t.Errorf("No subscribers found for event type 'testEventUnsub'")
	}
	subscriber := subscribers[0]

	EventBus.Unsubscribe(subscriber)

	select {
	case _, ok := <-subscriber.Ch:
		if ok {
			t.Errorf("Subscriber's channel should be closed, but it's not")
		}
	default:
	}
}

func TestSubscriberAlreadyExist(t *testing.T) {
	handler := &moduleCiHandler{}
	EventBus := NewEventBus()
	EventBus.StartSubscriber("testModuleSubExist", "testEventSubExist", 3, handler)

	exists := EventBus.subscriberExist("testEventSubExist", "testModuleSubExist")
	if !exists {
		t.Errorf("subscriberExist should return true for existing subscriber")
	}

	subscribers := EventBus.GetSubscribers("testEventSubExist")
	for _, sub := range subscribers {
		if sub.Name == "testModuleSubExist" {
			EventBus.Unsubscribe(sub)
		}
	}
}
