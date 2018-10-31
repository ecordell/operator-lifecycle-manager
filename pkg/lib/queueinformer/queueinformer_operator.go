package queueinformer

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/operatorclient"
	"github.com/pkg/errors"
)

// An Operator is a collection of QueueInformers
// OpClient is used to establish the connection to kubernetes
type Operator struct {
	queueInformers []*QueueInformer
	informers []cache.SharedIndexInformer
	OpClient       operatorclient.ClientInterface
}

// NewOperator creates a new Operator configured to manage the cluster defined in kubeconfig.
func NewOperator(kubeconfig string, queueInformers ...*QueueInformer) (*Operator, error) {
	opClient := operatorclient.NewClientFromConfig(kubeconfig)
	if queueInformers == nil {
		queueInformers = []*QueueInformer{}
	}
	operator := &Operator{
		OpClient:       opClient,
		queueInformers: queueInformers,
	}
	return operator, nil
}

func NewOperatorFromClient(opClient operatorclient.ClientInterface, queueInformers ...*QueueInformer) (*Operator, error) {
	if queueInformers == nil {
		queueInformers = []*QueueInformer{}
	}
	operator := &Operator{
		OpClient:       opClient,
		queueInformers: queueInformers,
	}
	return operator, nil
}

// RegisterQueueInformer adds a QueueInformer to this operator
func (o *Operator) RegisterQueueInformer(queueInformer *QueueInformer) {
	if o.queueInformers == nil {
		o.queueInformers = []*QueueInformer{}
	}
	o.queueInformers = append(o.queueInformers, queueInformer)
}

// RegisterInformer adds an Informer to this operator
func (o *Operator) RegisterInformer(informer cache.SharedIndexInformer) {
	if o.informers == nil {
		o.informers = []cache.SharedIndexInformer{}
	}
	o.informers = append(o.informers, informer)
}

// Run starts the operator's control loops
func (o *Operator) Run(stopc <-chan struct{}) (ready, done chan struct{}) {
	ready = make(chan struct{})
	done = make(chan struct{})

	go func() {
		defer func() {
			close(ready)
			close(done)
		}()

		for _, queueInformer := range o.queueInformers {
			defer queueInformer.queue.ShutDown()
		}

		errChan := make(chan error)
		go func() {
			v, err := o.OpClient.KubernetesInterface().Discovery().ServerVersion()
			if err != nil {
				errChan <- errors.Wrap(err, "communicating with server failed")
				return
			}
			log.Infof("connection established. cluster-version: %v", v)
			errChan <- nil
		}()

		var hasSyncedCheckFns []cache.InformerSynced
		for _, queueInformer := range o.queueInformers {
			hasSyncedCheckFns = append(hasSyncedCheckFns, queueInformer.informer.HasSynced)
		}
		for _, informer := range o.informers {
			hasSyncedCheckFns = append(hasSyncedCheckFns, informer.HasSynced)
		}

		select {
		case err := <-errChan:
			if err != nil {
				log.Infof("operator not ready: %s", err.Error())
				return
			}
			log.Info("operator ready")
		case <-stopc:
			return
		}

		log.Info("starting informers...")
		for _, queueInformer := range o.queueInformers {
			go queueInformer.informer.Run(stopc)
		}

		for _, informer := range o.informers {
			go informer.Run(stopc)
		}

		log.Info("waiting for caches to sync...")
		if ok := cache.WaitForCacheSync(stopc, hasSyncedCheckFns...); !ok {
			log.Info("failed to wait for caches to sync")
			return
		}

		log.Info("starting workers...")
		for _, queueInformer := range o.queueInformers {
			go o.worker(queueInformer)
		}
		ready <- struct{}{}
		<-stopc
	}()

	return
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (o *Operator) worker(loop *QueueInformer) {
	for o.processNextWorkItem(loop) {
	}
}

func (o *Operator) processNextWorkItem(loop *QueueInformer) bool {
	queue := loop.queue
	key, quit := queue.Get()

	if quit {
		return false
	}
	defer queue.Done(key)

	// requeue five times on error
	if err := o.sync(loop, key.(string)); err != nil && queue.NumRequeues(key.(string)) < 5 {
		log.Infof("retrying %s", key)
		utilruntime.HandleError(errors.Wrap(err, fmt.Sprintf("Sync %q failed", key)))
		queue.AddRateLimited(key)
		return true
	}
	queue.Forget(key)
	if err := loop.HandleMetrics(); err != nil {
		log.Error(err)
	}
	return true
}

func (o *Operator) sync(loop *QueueInformer, key string) error {
	logger := log.WithField("queue", loop.name).WithField("key", key)
	logger.Info("getting from queue")
	obj, exists, err := loop.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		// For now, we ignore the case where an object used to exist but no longer does
		logger.Info("couldn't get from queue")
		logger.Debugf("have keys: %v", loop.informer.GetIndexer().ListKeys())
		return nil
	}
	return loop.syncHandler(obj)
}
