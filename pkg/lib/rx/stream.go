package rx

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// MapFn returns a new object
type MapFn func(event EventInterface) EventInterface

// FilterFn returns bool as to whether the event should be included in the stream
type FilterFn func(event EventInterface) bool

// ConsumeFn does something with an event
type ConsumeFn func(event EventInterface)

var Canceled = fmt.Errorf("canceled")

// Generator knows how to get the next object
type Generator interface {
	Next() EventInterface
	Done() chan struct{}
}

// StreamInterface defines common stream interactions
type StreamInterface interface {
	Events() chan EventInterface
	Cancel()
	Err() chan error
	Wait() error
	Start()
	Map(fn MapFn) StreamInterface
	Filter(fn FilterFn) StreamInterface
	With(stream StreamInterface) StreamInterface
}

// baseStream defines common stream methods and data. Not to be used directly, baseStream does not satisfy
// StreamInterface.
type baseStream struct {
	events chan EventInterface
	cancel chan struct{}
	errc   chan error
}

func (s baseStream) Events() chan EventInterface {
	return s.events
}

func (s baseStream) Err() chan error {
	return s.errc
}

func (s baseStream) Wait() error {
	return <-s.errc
}

func (s baseStream) Cancel() {
	s.cancel <- struct{}{}
}

type ConsumerInterface interface {
	Consume(stream StreamInterface)
	Wait() error
}

type Consumer struct {
	fn     ConsumeFn
	stream StreamInterface
	errc   chan error
}

func (c Consumer) Consume() {
	go func() {
		for {
			select {
			case e, open := <-c.stream.Events():
				if open {
					log.Infof("consuming: %v", e)
					c.fn(e)
				} else {
					log.Warn("stream closed")
				}
			case err := <-c.stream.Err():
				log.Info("stream error: " + err.Error())
				c.errc <- err
				return
			}
		}
	}()
	c.stream.Start()
}

func (c Consumer) Wait() error {
	return <-c.errc
}

func NewConsumer(stream StreamInterface, fn ConsumeFn) Consumer {
	return Consumer{
		fn:     fn,
		stream: stream,
		errc:   make(chan error, 1),
	}
}

func NewBaseStream() baseStream {
	return baseStream{
		events: make(chan EventInterface),
		cancel: make(chan struct{}, 1),
		errc:   make(chan error, 1),
	}
}

// SourceInterface represents a source stream, which is a self-generating stream
type SourceInterface interface {
	Generator
	StreamInterface
}

// Source implements SourceInterface
type Source struct {
	baseStream
	generator Generator
}

// NewSource creates a Source stream from a generator
func NewSource(generator Generator) Source {
	return Source{
		baseStream: NewBaseStream(),
		generator:  generator,
	}
}

func (s Source) Map(fn MapFn) StreamInterface {
	return NewStream(&s, fn)
}

func (s Source) Filter(fn FilterFn) StreamInterface {
	return NewFilterStream(&s, fn)
}

func (s Source) With(stream StreamInterface) StreamInterface {
	return NewWithStream(&s, stream)
}

func (s Source) Start() {
	go func() {
		defer close(s.Events())
		canceled := false
	EVENTS:
		for {
			select {
			case <-s.cancel:
				log.Info("cancel")
				canceled = true
				break EVENTS
			case <-s.generator.Done():
				log.Info("generator done")
				break EVENTS
			default:
				if canceled {
					break EVENTS
				}
				log.Info("GET NEXT")
				next := s.generator.Next()
				log.Infof("next: %s", next)
				s.Events() <- next
			}
		}
		if canceled {
			s.errc <- Canceled
		} else {
			s.errc <- nil
		}
	}()
}

type Stream struct {
	baseStream
	source StreamInterface
	fn     MapFn
}

func NewStream(source StreamInterface, fn MapFn) *Stream {
	return &Stream{
		baseStream: NewBaseStream(),
		source:     source,
		fn:         fn,
	}
}

func (p Stream) Start() {
	go func() {
		defer close(p.events)
		canceled := false
		for e := range p.source.Events() {
			log.Infof("stream processing %v", e)
			select {
			case <-p.cancel:
				// cancelled, so cancel source as well
				log.Infof("cancelled processing %v", e)
				canceled = true
				p.source.Cancel()
			default:
				log.Infof("default %v", e)
				// process events
				if !canceled {
					log.Infof("not cancelled %v", e)
					p.events <- p.fn(e)
				}
			}
		}
		// source closed, so get the error from source (if any)
		p.errc <- p.source.Wait()
	}()
	p.source.Start()
}

func (p Stream) Map(fn MapFn) StreamInterface {
	return NewStream(&p, fn)
}

func (p Stream) Filter(fn FilterFn) StreamInterface {
	return NewFilterStream(&p, fn)
}

func (p Stream) With(stream StreamInterface) StreamInterface {
	return NewWithStream(&p, stream)
}

type FilterStream struct {
	baseStream
	source StreamInterface
	fn     FilterFn
}

func NewFilterStream(source StreamInterface, fn FilterFn) *FilterStream {
	return &FilterStream{
		baseStream: NewBaseStream(),
		source:     source,
		fn:         fn,
	}
}

func (p FilterStream) Start() {
	go func() {
		defer close(p.events)
		canceled := false
		for e := range p.source.Events() {
			select {
			case <-p.cancel:
				// cancelled, so cancel source as well
				canceled = true
				p.source.Cancel()
			default:
				// process events
				if !canceled {
					if p.fn(e) {
						p.events <- e
					}
				}
			}
		}
		// source closed, so get the error from source (if any)
		p.errc <- p.source.Wait()
	}()
	p.source.Start()
}

func (p FilterStream) Map(fn MapFn) StreamInterface {
	return NewStream(&p, fn)
}

func (p FilterStream) Filter(fn FilterFn) StreamInterface {
	return NewFilterStream(&p, fn)
}

func (p FilterStream) With(stream StreamInterface) StreamInterface {
	return NewWithStream(&p, stream)
}

type WithStream struct {
	baseStream
	lStream StreamInterface
	rStream StreamInterface
}

func NewWithStream(lStream StreamInterface, rStream StreamInterface) StreamInterface {
	return &WithStream{
		baseStream: NewBaseStream(),
		lStream:    lStream,
		rStream:    rStream,
	}
}

func (p WithStream) Start() {
	cancelc := make(chan struct{}, 2)
	errc := make(chan error, 2)
	go func() {
		canceled := false
		for {
			select {
			case <-p.cancel:
				canceled = true
				cancelc <- struct{}{}
			case e, open := <-p.lStream.Events():
				if open && !canceled {
					p.events <- e
				}
			case err := <-p.lStream.Err():
				errc <- err
				return
			}
		}
	}()

	go func() {
		canceled := false
		for {
			select {
			case <-p.cancel:
				canceled = true
				cancelc <- struct{}{}
			case e, open := <-p.rStream.Events():
				if open && !canceled {
					p.events <- e
				}
			case err := <-p.rStream.Err():
				errc <- err
				return
			}
		}
	}()

	go func() {
		// one of the streams has reported p as canceled
		if _, open := <-cancelc; open {
			p.lStream.Cancel()
			p.rStream.Cancel()
		}
		// will return when cancelc is closed
	}()

	go func() {
		defer close(p.events)
		defer close(cancelc)
		err1 := <-errc
		err2 := <-errc
		if err1 != nil {
			p.errc <- err1
			return
		}
		p.errc <- err2
	}()

	p.lStream.Start()
	p.rStream.Start()
}

func (p WithStream) Map(fn MapFn) StreamInterface {
	return NewStream(&p, fn)
}

func (p WithStream) Filter(fn FilterFn) StreamInterface {
	return NewFilterStream(&p, fn)
}

func (p WithStream) With(stream StreamInterface) StreamInterface {
	return NewWithStream(&p, stream)
}

// Events represent some action or change in state of the system
type EventInterface interface {
	GetObj() interface{}
	SetObj(interface{})
	Key() string
	EventType() string
}

type Event struct {
	obj       interface{}
	key       string
	eventType string
}

func (k Event) GetObj() interface{} {
	return k.obj
}

func (k Event) SetObj(o interface{}) {
	k.obj = o
	return
}

func (k Event) Key() string {
	return k.key
}

func (k Event) EventType() string {
	return k.eventType
}

var EmptyEvent = Event{obj: nil, key: "", eventType: "None"}

type KubeEventType string

const (
	KubeEventTypeAdd    KubeEventType = "Add"
	KubeEventTypeUpdate KubeEventType = "Update"
	KubeEventTypeDelete KubeEventType = "Delete"
)

type KubeEvent struct {
	queueKey  string
	object    interface{}
	oldObject interface{}
	eventType KubeEventType
}

func (k KubeEvent) GetObj() interface{} {
	return k.object
}

func (k KubeEvent) SetObj(o interface{}) {
	k.object = o
	return
}

func (k KubeEvent) Key() string {
	return k.queueKey
}

func (k KubeEvent) EventType() string {
	return string(k.eventType)
}

// KubeStreams are event streams that come from the informers
type KubeStream struct {
	baseStream
	informer cache.SharedIndexInformer
}

func (k KubeStream) Start() {
	informerStopC := make(chan struct{})
	go k.informer.Run(informerStopC)
	log.Info("waiting for caches to sync...")

	if !cache.WaitForCacheSync(informerStopC, k.informer.HasSynced) {
		k.errc <- fmt.Errorf("caches didn't sync")
		return
	}

	go func() {
		defer close(k.Events())
		canceled := false
	EVENTS:
		for {
			select {
			case <-k.cancel:
				log.Info("stream cancelled")
				canceled = true
				break EVENTS
			default:
				if canceled {
					log.Info("stream cancelling")
					break EVENTS
				}
			}
		}
		log.Info("only here after cancel")
		informerStopC <- struct{}{}
		if canceled {
			k.errc <- Canceled
		} else {
			k.errc <- nil
		}
	}()
}

func (k KubeStream) Map(fn MapFn) StreamInterface {
	return NewStream(&k, fn)
}

func (k KubeStream) Filter(fn FilterFn) StreamInterface {
	return NewFilterStream(&k, fn)
}

func (k KubeStream) With(stream StreamInterface) StreamInterface {
	return NewWithStream(&k, stream)
}

// keyFunc turns an object into a key for the queue. In the future will use a (name, namespace) struct as key
func (k KubeStream) keyFunc(obj interface{}) (string, bool) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return key, false
	}

	return key, true
}

func (k KubeStream) OnAdd(obj interface{}) {
	log.Infof("Adding %v", obj)
	key, ok := k.keyFunc(obj)
	if !ok {
		return
	}
	k.events <- KubeEvent{
		queueKey:  key,
		object:    obj,
		oldObject: nil,
		eventType: KubeEventTypeAdd,
	}
}
func (k KubeStream) OnUpdate(oldObj, newObj interface{}) {
	log.Infof("updating %v", newObj)
	key, ok := k.keyFunc(newObj)
	if !ok {
		return
	}
	log.Info("adding to events")
	log.Infof("%#v", k.events)
	log.Infof("%#v", k)
	k.events <- KubeEvent{
		queueKey:  key,
		object:    newObj,
		oldObject: oldObj,
		eventType: KubeEventTypeUpdate,
	}
	log.Info("added")
}
func (k KubeStream) OnDelete(obj interface{}) {
	log.Infof("deleting %v", obj)
	key, ok := k.keyFunc(obj)
	if !ok {
		return
	}
	k.events <- KubeEvent{
		queueKey:  key,
		object:    obj,
		oldObject: nil,
		eventType: KubeEventTypeDelete,
	}
}

func NewKubeStream(informer cache.SharedIndexInformer) KubeStream {
	s := KubeStream{
		baseStream: NewBaseStream(),
		informer:   informer,
	}
	s.informer.AddEventHandler(s)
	return s
}

// QueueKubeStreams are event streams that come from the informers filtered through a workqueue
type QueueKubeStream struct {
	StreamInterface
	queue workqueue.RateLimitingInterface
}

func NewQueueKubeStream(kubeStream KubeStream, queue workqueue.RateLimitingInterface) QueueKubeStream {
	q := QueueKubeStream{
		queue: queue,
	}
	q.StreamInterface = kubeStream.Map(q.enqueueEvents)
	return q
}

func (q QueueKubeStream) enqueueEvents(event EventInterface) EventInterface {
	log.Infof("enqueueEvents: %v", event)
	kevent, ok := event.(KubeEvent)
	log.Info(ok)
	if !ok {
		return nil
	}
	log.Info("queueing event: %v", kevent)
	switch KubeEventType(kevent.EventType()) {
	case KubeEventTypeAdd:
		q.queue.AddRateLimited(kevent)
	case KubeEventTypeUpdate:
		q.queue.AddRateLimited(kevent)
	case KubeEventTypeDelete:
		q.queue.Forget(kevent)
	}
	return kevent
}

type QueueKubeGenerator struct {
	done  chan struct{}
	queue workqueue.RateLimitingInterface
}

func (g QueueKubeGenerator) Next() EventInterface {
	log.Info("trying to get next from queue")
	key, quit := g.queue.Get()
	log.Info("generating from queue: %s", key)
	defer g.queue.Done(key)
	if quit {
		log.Errorf("queue shutting down")
		g.Done() <- struct{}{}
		g.queue.ShutDown()
	}

	kevent, ok := key.(KubeEvent)
	if !ok {
		log.Errorf("coudln't get kevent")
		return EmptyEvent
	}

	g.queue.Forget(key)
	return kevent
}

func (g QueueKubeGenerator) Done() chan struct{} {
	return g.done
}

type QueueSource struct {
	Source
	// streams are a set of streams that feed into the queue
	streams []QueueKubeStream
}

// NewQueueSource creates a Stream that reads from a workqueue for its events
func NewQueueSource(streams []QueueKubeStream, queue workqueue.RateLimitingInterface) QueueSource {
	return QueueSource{
		Source: NewSource(QueueKubeGenerator{
			done:  make(chan struct{}, 1),
			queue: queue,
		}),
		streams: streams,
	}
}

func (s QueueSource) Start() {
	for _, stream := range s.streams {
		go stream.Start()
		// streams need a watcher to continue writing
		NewConsumer(stream, func(event EventInterface) {
			return
		}).Consume()
	}
	s.Source.Start()
}

// NewSimpleKubeSource creates a queue-backed source just from an informer
func NewSimpleKubeSource(informers []cache.SharedIndexInformer, queueName string) QueueSource {
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), queueName)
	var streams []QueueKubeStream
	for _, informer := range informers {
		streams = append(streams, NewQueueKubeStream(NewKubeStream(informer), queue))
	}
	return NewQueueSource(streams, queue)

}

// NewKubeSourceForQueue creates a queue-backed source just from an informer and an existing queue
func NewKubeSourceForQueue(informers []cache.SharedIndexInformer, queue workqueue.RateLimitingInterface) QueueSource {
	var streams []QueueKubeStream
	for _, informer := range informers {
		streams = append(streams, NewQueueKubeStream(NewKubeStream(informer), queue))
	}
	return NewQueueSource(streams, queue)
}
