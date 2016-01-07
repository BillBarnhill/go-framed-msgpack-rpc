package rpc

import (
	"golang.org/x/net/context"
)

type request interface {
	RPCMessage
	CancelFunc() context.CancelFunc
	Reply(encoder, interface{}, interface{}) error
	Serve(encoder, *ServeHandlerDescription, WrapErrorFunc)
	LogInvocation(err error)
	LogCompletion(res interface{}, err error)
}

type requestImpl struct {
	RPCMessage
	ctx        context.Context
	cancelFunc context.CancelFunc
	log        LogInterface
}

func (req *requestImpl) CancelFunc() context.CancelFunc {
	return req.cancelFunc
}

func (r *requestImpl) LogInvocation(error)                           {}
func (r *requestImpl) LogCompletion(interface{}, error)              {}
func (r *requestImpl) Reply(encoder, interface{}, interface{}) error { return nil }
func (r *requestImpl) Serve(encoder, *ServeHandlerDescription, WrapErrorFunc) {
}

type callRequest struct {
	requestImpl
}

func newCallRequest(rpc RPCMessage, log LogInterface) *callRequest {
	ctx, cancel := context.WithCancel(context.Background())
	return &callRequest{
		requestImpl: requestImpl{
			RPCMessage: rpc,
			ctx:        ctx,
			cancelFunc: cancel,
			log:        log,
		},
	}
}

func (r *callRequest) LogInvocation(err error) {
	r.log.ServerCall(r.SeqNo(), r.Name(), err, r.Arg())
}

func (r *callRequest) LogCompletion(res interface{}, err error) {
	r.log.ServerReply(r.SeqNo(), r.Name(), err, res)
}

func (r *callRequest) Reply(enc encoder, res interface{}, errArg interface{}) (err error) {
	select {
	case <-r.ctx.Done():
		err = newCanceledError(r.Name(), r.SeqNo())
		r.log.Info(err.Error())
	default:
		v := []interface{}{
			MethodResponse,
			r.SeqNo(),
			errArg,
			res,
		}
		errCh := enc.Encode(v)
		// TODO investigate hypothetical server-side hang here
		err = <-errCh
		if err != nil {
			r.log.Warning("Reply error for %d: %s", r.SeqNo(), err.Error())
		}
	}
	return err
}

func (r *callRequest) Serve(transmitter encoder, handler *ServeHandlerDescription, wrapErrorFunc WrapErrorFunc) {

	prof := r.log.StartProfiler("serve %s", r.Name())
	arg := r.Arg()

	r.LogInvocation(nil)
	res, err := handler.Handler(r.ctx, arg)
	prof.Stop()
	r.LogCompletion(res, err)

	r.Reply(transmitter, res, wrapError(wrapErrorFunc, err))
}

type notifyRequest struct {
	requestImpl
}

func newNotifyRequest(rpc RPCMessage, log LogInterface) *notifyRequest {
	ctx, cancel := context.WithCancel(context.Background())
	return &notifyRequest{
		requestImpl: requestImpl{
			RPCMessage: rpc,
			ctx:        ctx,
			cancelFunc: cancel,
			log:        log,
		},
	}
}

func (r *notifyRequest) LogInvocation(err error) {
	r.log.ServerNotifyCall(r.Name(), err, r.Arg())
}

func (r *notifyRequest) LogCompletion(_ interface{}, err error) {
	r.log.ServerNotifyComplete(r.Name(), err)
}

func (r *notifyRequest) Serve(transmitter encoder, handler *ServeHandlerDescription, wrapErrorFunc WrapErrorFunc) {

	prof := r.log.StartProfiler("serve-notify %s", r.Name())
	arg := r.Arg()

	r.LogInvocation(nil)
	_, err := handler.Handler(r.ctx, arg)
	prof.Stop()
	r.LogCompletion(nil, err)
}
