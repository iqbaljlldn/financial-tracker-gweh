package observability

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/propagation"
)

// TraceContextKey is the reserved top-level payload key under which the W3C trace
// context of the originating operation is stored in every outbox_events.payload.
const TraceContextKey = "traceContext"

var propagator = propagation.TraceContext{}

// InjectTraceContext serializes the span context carried by ctx into a W3C carrier map.
// It returns an empty map when ctx carries no valid span context.
func InjectTraceContext(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	return map[string]string(carrier)
}

// ExtractTraceContext continues a trace persisted by InjectTraceContext, returning a
// context whose remote span context matches the stored carrier (17_observability.md §5.2).
func ExtractTraceContext(ctx context.Context, carrier map[string]string) context.Context {
	if len(carrier) == 0 {
		return ctx
	}
	return propagator.Extract(ctx, propagation.MapCarrier(carrier))
}

// EmbedTraceContext marshals payload to JSON and merges the trace context of ctx into the
// top-level object under TraceContextKey at write time [GATE: 17_observability.md §5.2].
// Non-object payloads are wrapped as {"data": <payload>, "traceContext": {...}}.
func EmbedTraceContext(ctx context.Context, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	obj := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		obj = map[string]json.RawMessage{"data": raw}
	}

	tc, err := json.Marshal(InjectTraceContext(ctx))
	if err != nil {
		return nil, err
	}
	obj[TraceContextKey] = tc

	return json.Marshal(obj)
}

// TraceContextFromPayload extracts the stored carrier from a raw outbox payload, if present.
func TraceContextFromPayload(raw []byte) map[string]string {
	var envelope struct {
		TraceContext map[string]string `json:"traceContext"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	return envelope.TraceContext
}
