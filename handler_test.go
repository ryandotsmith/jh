package jh

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlers(t *testing.T) {
	type addReq struct {
		X, Y int
	}
	type addResp struct {
		Sum int
	}
	add := func(ctx context.Context, r addReq) (addResp, error) {
		return addResp{r.X + r.Y}, nil
	}

	var (
		body = strings.NewReader(`{"X": 1, "Y": 1}`)
		r    = httptest.NewRequest("POST", "/", body)
		rec  = httptest.NewRecorder()
	)
	h, _ := Handler(add, ErrHandler)
	h.ServeHTTP(rec, r)

	got, _ := ioutil.ReadAll(rec.Result().Body)
	want := "{\"Sum\":2}\n"
	if string(got) != want {
		t.Errorf("got = %q; want %q", got, want)
	}
}

func TestErrHandler(t *testing.T) {
	var (
		ctx = context.Background()
		rec = httptest.NewRecorder()
		err = Error{Code: 420, Message: "m"}
	)
	ErrHandler(ctx, rec, err)

	var (
		gotbody, _ = ioutil.ReadAll(rec.Result().Body)
		wantbody   = "{\"message\":\"m\"}\n"
	)
	if string(gotbody) != wantbody {
		t.Errorf("got %q want %q", gotbody, wantbody)
	}
	var (
		gotstatus  = rec.Result().StatusCode
		wantstatus = 420
	)
	if gotstatus != wantstatus {
		t.Errorf("got %d want %d", gotstatus, wantstatus)
	}
}
