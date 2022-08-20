/*
	This package helps you encode/decode JSON from HTTP request/response
	into Go structs.

		type req struct {
			X, Y int
		}

		type resp struct {
			Sum int
		}

		func add(ctx context.Context, r req) (*resp, error) {
			return &resp{r.X+r.Y}, nil
		}

		http.Handle("/add", Handler(add), ErrHandler)


	In this example, add is a wrapped function that jh
	will use to determine how to encode/decode json.
*/
package jh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

type key int

const (
	reqKey = iota
	respKey
)

// Can be used inside of a wrapped function.
// eg parsing url parameters
func Request(ctx context.Context) *http.Request {
	return ctx.Value(reqKey).(*http.Request)
}

// Can be used inside of a wrapped function.
// eg setting a response header
func ResponseWriter(ctx context.Context) http.ResponseWriter {
	return ctx.Value(respKey).(http.ResponseWriter)
}

type handler struct {
	f  reflect.Value
	ef func(context.Context, http.ResponseWriter, error)
}

type Error struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
}

func (e Error) Error() string {
	return fmt.Sprintf("jh: %s", e.Message)
}

func ErrHandler(ctx context.Context, w http.ResponseWriter, err error) {
	var jhe Error
	if errors.As(err, &jhe) {
		w.WriteHeader(jhe.Code)
		json.NewEncoder(w).Encode(jhe)
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(&struct {
		Messages string `json:"error"`
	}{err.Error()})
}

var (
	ErrTooFewArgs  = errors.New("jh: handler: too few args. expected wrappedFunc with at least 1 arg")
	ErrTooManyArgs = errors.New("jh: handler: too many args. expected wrappedFunc with no more than 2 args")
	ErrMissingCtx  = errors.New("jh: handler: 1st arg must be context.Context")
	ErrNumRet      = errors.New("jh: handler: expected wrappedFunc to have 2 return values")
	ErrMissingErr  = errors.New("jh: handler: wrappedFunc's 2nd return value must be an error")
)

// Reflection is used on wrappedFunc to determine the req/resp
// types for later json encoding/decoding.
// An error is returned when wrappedFunc doesn't conform to one of the
// following forms:
//		func(context.Context, struct{}) (*struct{}, error)
//		func(context.Context) (*struct{}, error)
//
// errFunc is called when a wrappedFunc returns an error or
// when json encoding/decdoing encounters an error.
func Handler(
	wrappedFunc any,
	errFunc func(context.Context, http.ResponseWriter, error),
) (http.Handler, error) {
	var f = reflect.ValueOf(wrappedFunc)

	if f.Type().NumIn() > 2 {
		return nil, ErrTooManyArgs
	}
	if f.Type().NumIn() < 1 {
		return nil, ErrTooFewArgs
	}
	if f.Type().NumOut() != 2 {
		return nil, ErrNumRet
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !f.Type().In(0).Implements(contextType) {
		return nil, ErrMissingCtx
	}
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !f.Type().Out(1).Implements(errorType) {
		return nil, ErrMissingErr
	}

	return &handler{
		f:  f,
		ef: errFunc,
	}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, reqKey, r)
	ctx = context.WithValue(ctx, respKey, w)

	var ret []reflect.Value
	switch h.f.Type().NumIn() {
	case 1:
		ret = h.f.Call([]reflect.Value{reflect.ValueOf(ctx)})
	case 2:
		var i = reflect.New(h.f.Type().In(1))
		err := json.NewDecoder(r.Body).Decode(i.Interface())
		if err != nil {
			h.ef(ctx, w, Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}
		ret = h.f.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			i.Elem(),
		})
	}

	// should never happen since
	// since we check the length of f's output list in [Handler]
	if len(ret) != 2 {
		h.ef(ctx, w, errors.New("handler needs 2 return values"))
		return
	}

	err, _ := ret[1].Interface().(error)
	if err != nil {
		h.ef(ctx, w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ret[0].Interface())
}
