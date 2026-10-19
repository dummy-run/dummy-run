package bidi

import (
	"reflect"
	"testing"
)

func TestMergeHeaders(t *testing.T) {
	original := []Header{
		StringHeader("Host", "example.test"),
		StringHeader("Accept", "*/*"),
		StringHeader("traceparent", "00-aaaa-bbbb-01"),
	}
	set := []Header{
		StringHeader("baggage", "dummyrun.run.id=r1"),
		StringHeader("Traceparent", "00-cccc-dddd-01"),
	}
	got := MergeHeaders(original, set)
	want := []Header{
		StringHeader("Host", "example.test"),
		StringHeader("Accept", "*/*"),
		StringHeader("Traceparent", "00-cccc-dddd-01"),
		StringHeader("baggage", "dummyrun.run.id=r1"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge gave %v, want %v", got, want)
	}
	if v, ok := HeaderValue(got, "TRACEPARENT"); !ok || v != "00-cccc-dddd-01" {
		t.Errorf("HeaderValue gave %q %v", v, ok)
	}
	if _, ok := HeaderValue(got, "cookie"); ok {
		t.Error("HeaderValue found a header that is not there")
	}
}

func TestMergeHeadersKeepsOriginalWhenNothingSet(t *testing.T) {
	original := []Header{StringHeader("Host", "a")}
	if got := MergeHeaders(original, nil); !reflect.DeepEqual(got, original) {
		t.Errorf("got %v", got)
	}
	if got := MergeHeaders(nil, nil); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestMergeHeadersDropsDuplicateOriginals(t *testing.T) {
	original := []Header{StringHeader("baggage", "a=1"), StringHeader("baggage", "b=2")}
	got := MergeHeaders(original, []Header{StringHeader("baggage", "c=3")})
	if len(got) != 1 || got[0].Value.Value != "c=3" {
		t.Errorf("got %v", got)
	}
}
