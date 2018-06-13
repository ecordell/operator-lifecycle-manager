package rx

import (
	"fmt"
	"runtime"
	"strconv"
	"testing"

	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type TestEvent struct {
	val string
}

func (e TestEvent) GetObj() interface{} {
	return e.val
}

func (e TestEvent) SetObj(obj interface{}) {
	e.val = obj.(string)
}

func (e TestEvent) Key() string {
	return e.val
}

func (e TestEvent) EventType() string {
	return "Test"
}

var _ EventInterface = &TestEvent{}

func ExpectEvents(t *testing.T, stream StreamInterface, expected []EventInterface) {
	go func() {
		i := 0
		for e := range stream.Events() {
			require.Equal(t, expected[i], e)
			i += 1
		}
		require.Equal(t, len(expected), i)
	}()
}

type ArrayStreamGenerator struct {
	events       []TestEvent
	currentEvent int
	done         chan struct{}
}

func (g *ArrayStreamGenerator) Next() EventInterface {
	event := g.events[g.currentEvent]
	g.currentEvent += 1
	if g.currentEvent >= len(g.events) {
		g.Done() <- struct{}{}
	}
	return event
}

func (g *ArrayStreamGenerator) Done() chan struct{} {
	return g.done
}

func NewArrayStreamGenerator(events []TestEvent) Generator {
	return &ArrayStreamGenerator{
		events: events,
		done:   make(chan struct{}, 1),
	}
}

type InfiniteStreamGenerator struct {
	done chan struct{}
	val  string
}

func (g InfiniteStreamGenerator) Next() EventInterface {
	return TestEvent{g.val}
}

func (g InfiniteStreamGenerator) Done() chan struct{} {
	return g.done
}

func NewInfiniteStreamGenerator(val string) InfiniteStreamGenerator {
	return InfiniteStreamGenerator{
		done: make(chan struct{}, 1),
		val:  val,
	}
}

func DetectGoroutineLeak(t *testing.T, grCount int) {
	runtime.GC()
	buf := make([]byte, 1<<20)
	runtime.Stack(buf, true)
	require.Equal(t, grCount, runtime.NumGoroutine(), "wrong number of goroutines:\n%s", string(buf))
}

func CancelAfter(events chan EventInterface, stream StreamInterface, numEvents int) {
	go func(stopnum int) {
		i := 0
		for range events {
			i += 1
			if i == stopnum {
				stream.Cancel()
			}
		}
	}(numEvents)
}

func TestSource(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	events := []TestEvent{{"1"}, {"2"}}
	expected := []EventInterface{TestEvent{"1"}, TestEvent{"2"}}
	source := NewSource(NewArrayStreamGenerator(events))
	source.Start()
	ExpectEvents(t, source, expected)
	require.Nil(t, source.Wait())
}

func TestSourceCancel(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	source := NewSource(NewInfiniteStreamGenerator(""))
	source.Start()
	CancelAfter(source.Events(), source, 4)
	require.EqualError(t, source.Wait(), Canceled.Error())
}

func TestStreamConsumer(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	events := []TestEvent{{"1"}, {"2"}}
	expected := []EventInterface{TestEvent{"1"}, TestEvent{"2"}}
	source := NewSource(NewArrayStreamGenerator(events))
	i := 0
	consumer := NewConsumer(source, func(e EventInterface) {
		require.Equal(t, expected[i], e)
		i += 1
	})
	consumer.Consume()
	require.Nil(t, consumer.Wait())
	require.Equal(t, 2, i)
}

func TestStreamConsumerCancel(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	source := NewSource(NewInfiniteStreamGenerator(""))
	CancelAfter(source.Events(), source, 4)
	consumer := NewConsumer(source, func(e EventInterface) {
		fmt.Println("process")
	})
	consumer.Consume()
	require.EqualError(t, consumer.Wait(), Canceled.Error())
}

func TestMap(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	events := []TestEvent{{"1"}, {"2"}}
	expected := []EventInterface{TestEvent{"2"}, TestEvent{"4"}}
	source := NewSource(NewArrayStreamGenerator(events))
	stream := source.Map(func(event EventInterface) EventInterface {
		te := event.(TestEvent)
		i, _ := strconv.Atoi(te.val)
		return TestEvent{fmt.Sprintf("%d", i*2)}
	})
	stream.Start()
	ExpectEvents(t, stream, expected)
	require.Nil(t, stream.Wait())
}

func TestMapCancel(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []TestEvent{{}, {}}
	source := NewSource(NewInfiniteStreamGenerator(""))
	out := make(chan TestEvent, len(expected))
	stream := source.Map(func(event EventInterface) EventInterface {
		out <- event.(TestEvent)
		return event
	})

	CancelAfter(stream.Events(), stream, len(expected))

	stream.Start()
	require.EqualError(t, stream.Wait(), Canceled.Error())
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
}

func TestMapCancelSource(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []TestEvent{{}, {}, {}}
	source := NewSource(NewInfiniteStreamGenerator(""))
	out := make(chan TestEvent, len(expected))
	stream := source.Map(func(event EventInterface) EventInterface {
		out <- event.(TestEvent)
		return event
	})

	CancelAfter(stream.Events(), source, len(expected))

	stream.Start()
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
	require.EqualError(t, stream.Wait(), Canceled.Error())
}

func TestMapMap(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	events := []TestEvent{{"1"}, {"2"}}
	expected := []EventInterface{TestEvent{"1"}, TestEvent{"3"}}
	source := NewSource(NewArrayStreamGenerator(events))
	stream := source.Map(func(event EventInterface) EventInterface {
		te := event.(TestEvent)
		i, _ := strconv.Atoi(te.val)
		return TestEvent{fmt.Sprintf("%d", i*2)}
	}).Map(func(event EventInterface) EventInterface {
		te := event.(TestEvent)
		i, _ := strconv.Atoi(te.val)
		return TestEvent{fmt.Sprintf("%d", i-1)}
	})
	stream.Start()
	ExpectEvents(t, stream, expected)
	require.Nil(t, stream.Wait())
}

func TestMapMapCancel(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []TestEvent{{"mapped2"}, {"mapped2"}}
	source := NewSource(NewInfiniteStreamGenerator(""))
	out := make(chan TestEvent, len(expected))
	stream := source.Map(func(event EventInterface) EventInterface {
		return TestEvent{"mapped"}
	}).Map(func(event EventInterface) EventInterface {
		out <- TestEvent{"mapped2"}
		return TestEvent{"mapped2"}
	})

	CancelAfter(stream.Events(), stream, len(expected))

	stream.Start()
	require.EqualError(t, stream.Wait(), Canceled.Error())
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
}

func TestMapMapCancelSource(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []TestEvent{{"mapped2"}, {"mapped2"}}
	source := NewSource(NewInfiniteStreamGenerator(""))
	out := make(chan TestEvent, len(expected))
	stream := source.Map(func(event EventInterface) EventInterface {
		return TestEvent{"mapped"}
	}).Map(func(event EventInterface) EventInterface {
		out <- TestEvent{"mapped2"}
		return TestEvent{"mapped2"}
	})

	CancelAfter(stream.Events(), source, len(expected))

	stream.Start()
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
	require.EqualError(t, stream.Wait(), Canceled.Error())
}

func TestMapMapCancelIntermediate(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []TestEvent{{"mapped2"}, {"mapped2"}}
	source := NewSource(NewInfiniteStreamGenerator(""))
	out := make(chan TestEvent, len(expected))
	mapped := source.Map(func(event EventInterface) EventInterface {
		return TestEvent{"mapped"}
	})
	stream := mapped.Map(func(event EventInterface) EventInterface {
		out <- TestEvent{"mapped2"}
		return TestEvent{"mapped2"}
	})

	CancelAfter(stream.Events(), mapped, len(expected))

	stream.Start()
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
	require.EqualError(t, stream.Wait(), Canceled.Error())
}

func TestWith(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	levents := []TestEvent{{"1"}, {"2"}}
	revents := []TestEvent{{"3"}, {"4"}}
	expected := []EventInterface{TestEvent{"1"}, TestEvent{"2"}, TestEvent{"3"}, TestEvent{"4"}}
	lsource := NewSource(NewArrayStreamGenerator(levents))
	rsource := NewSource(NewArrayStreamGenerator(revents))

	stream := lsource.With(rsource)
	stream.Start()

	var events []TestEvent
	for e := range stream.Events() {
		events = append(events, e.(TestEvent))
	}

	require.ElementsMatch(t, expected, events)
	require.Nil(t, stream.Wait())
}

func TestWithCancel(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	expected := []EventInterface{TestEvent{}, TestEvent{}, TestEvent{}, TestEvent{}}
	out := make(chan TestEvent, len(expected))
	lsource := NewSource(NewInfiniteStreamGenerator("l"))
	rsource := NewSource(NewInfiniteStreamGenerator("r"))

	stream := lsource.With(rsource).Map(func(event EventInterface) EventInterface {
		out <- TestEvent{}
		return TestEvent{}
	})

	CancelAfter(stream.Events(), stream, len(expected))

	stream.Start()
	for _, e := range expected {
		require.Equal(t, e, <-out)
	}
	require.EqualError(t, stream.Wait(), Canceled.Error())
}

func TestWithWith(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	levents := []TestEvent{{"1"}, {"2"}}
	revents := []TestEvent{{"3"}, {"4"}}
	sevents := []TestEvent{{"5"}}
	expected := []EventInterface{TestEvent{"1"}, TestEvent{"2"}, TestEvent{"3"}, TestEvent{"4"}, TestEvent{"5"}}
	lsource := NewSource(NewArrayStreamGenerator(levents))
	rsource := NewSource(NewArrayStreamGenerator(revents))
	ssource := NewSource(NewArrayStreamGenerator(sevents))

	stream := lsource.With(rsource).With(ssource)
	stream.Start()

	var events []TestEvent
	for e := range stream.Events() {
		events = append(events, e.(TestEvent))
	}

	require.ElementsMatch(t, expected, events)
	require.Nil(t, stream.Wait())
}

func TestFilter(t *testing.T) {
	defer DetectGoroutineLeak(t, runtime.NumGoroutine())
	events := []TestEvent{{"1"}, {"2"}}
	expected := []EventInterface{TestEvent{"2"}}
	source := NewSource(NewArrayStreamGenerator(events))
	stream := source.Filter(func(event EventInterface) bool {
		te := event.(TestEvent)
		i, _ := strconv.Atoi(te.val)
		return i%2 == 0
	})
	stream.Start()
	ExpectEvents(t, stream, expected)
	require.Nil(t, stream.Wait())
}

func TestKubeStream(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceInformer := informers.NewSharedInformerFactory(kclient, 5*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceKubeStream := NewKubeStream(namespaceInformer)
	namespaceKubeStream.Start()

	var events []KubeEvent
	for e := range namespaceKubeStream.Events() {
		events = append(events, e.(KubeEvent))
		if len(events) == len(expected) {
			namespaceKubeStream.Cancel()
		}
	}
	require.EqualError(t, namespaceKubeStream.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)
	//
	//namespaceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "namespaces")
	//namespaceQueueSource := rx.NewQueueSource(namespaceQueue)
}

func TestConsumeKubeStream(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceInformer := informers.NewSharedInformerFactory(kclient, 5*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceKubeStream := NewKubeStream(namespaceInformer)

	var events []KubeEvent
	consumer := NewConsumer(namespaceKubeStream, func(event EventInterface) {
		events = append(events, event.(KubeEvent))
		if len(events) == len(expected) {
			namespaceKubeStream.Cancel()
		}
	})
	consumer.Consume()
	require.EqualError(t, consumer.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)
}

func TestQueueKubeStream(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "namespaces")
	namespaceInformer := informers.NewSharedInformerFactory(kclient, 5*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceKubeStream := NewKubeStream(namespaceInformer)
	namespaceQueueKubeStream := NewQueueKubeStream(namespaceKubeStream, namespaceQueue)
	namespaceQueueKubeStream.Start()

	var events []KubeEvent
	for e := range namespaceQueueKubeStream.Events() {
		events = append(events, e.(KubeEvent))
		if len(events) == len(expected) {
			namespaceKubeStream.Cancel()
		}
	}
	require.EqualError(t, namespaceQueueKubeStream.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)
}

func TestQueueSource(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceInformer := informers.NewSharedInformerFactory(kclient, 5*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceQueueSource := NewSimpleKubeSource([]cache.SharedIndexInformer{namespaceInformer}, "namsepaces")
	namespaceQueueSource.Start()

	var events []KubeEvent
	for e := range namespaceQueueSource.Events() {
		events = append(events, e.(KubeEvent))
		// len - 1 is a hack to stop at the right number of events since we're a layer away
		if len(events) == len(expected)-1 {
			namespaceQueueSource.Cancel()
		}
	}

	require.EqualError(t, namespaceQueueSource.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)

}

func TestConsumeQueueSource(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceInformer := informers.NewSharedInformerFactory(kclient, 5*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceQueueSource := NewSimpleKubeSource([]cache.SharedIndexInformer{namespaceInformer}, "namsepaces")
	namespaceQueueSource.Start()

	var events []KubeEvent
	consumer := NewConsumer(namespaceQueueSource, func(event EventInterface) {
		events = append(events, event.(KubeEvent))
		if len(events) == len(expected) {
			namespaceQueueSource.Cancel()
		}
	})
	consumer.Consume()

	require.EqualError(t, consumer.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)

}

func TestConsumerQueueFactory(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "testNS"}}
	kclient := fake.NewSimpleClientset(namespace)
	expected := []KubeEvent{
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: nil,
			eventType: KubeEventTypeAdd,
		},
		{
			queueKey:  "testNS",
			object:    namespace,
			oldObject: namespace,
			eventType: KubeEventTypeUpdate,
		},
	}

	namespaceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "namespaces")
	namespaceInformer := informers.NewSharedInformerFactory(kclient, 15*time.Millisecond).Core().V1().Namespaces().Informer()
	namespaceKubeStream := NewKubeStream(namespaceInformer)
	namespaceQueueKubeStream := NewQueueKubeStream(namespaceKubeStream, namespaceQueue)
	namespaceQueueSource := NewQueueSource([]QueueKubeStream{namespaceQueueKubeStream}, namespaceQueue)
	namespaceQueueSource.Start()

	var events []KubeEvent
	consumer := NewConsumer(namespaceQueueSource, func(event EventInterface) {
		events = append(events, event.(KubeEvent))
		if len(events) == len(expected) {
			namespaceQueueSource.Cancel()
		}
	})
	consumer.Consume()

	require.EqualError(t, consumer.Wait(), Canceled.Error())
	require.ElementsMatch(t, expected, events)

}
