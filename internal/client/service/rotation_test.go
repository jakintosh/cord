package service

import (
	"testing"
	"time"
)

var rotationBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNextCandidate_NoEndpoints(t *testing.T) {
	if _, ok := nextCandidate(nil, rotationBase); ok {
		t.Error("expected no candidate for empty endpoint list")
	}
}

func TestNextCandidate_NeverAttemptedPicksCatalogOrder(t *testing.T) {
	endpoints := []PeerEndpoint{
		{Endpoint: "1.1.1.1:51820"},
		{Endpoint: "2.2.2.2:51820"},
	}

	candidate, ok := nextCandidate(endpoints, rotationBase)
	if !ok {
		t.Fatal("expected a candidate")
	}
	if candidate != "1.1.1.1:51820" {
		t.Errorf("candidate = %q, want first catalog entry", candidate)
	}
}

func TestNextCandidate_DwellBlocksRecentAttempt(t *testing.T) {
	now := rotationBase
	endpoints := []PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", LastAttemptedAt: now.Add(-30 * time.Second)},
		{Endpoint: "2.2.2.2:51820"},
	}

	if _, ok := nextCandidate(endpoints, now); ok {
		t.Error("expected dwell to block rotation within RotateInterval")
	}
}

func TestNextCandidate_RoundRobinsLeastRecentlyAttempted(t *testing.T) {
	now := rotationBase
	endpoints := []PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", LastAttemptedAt: now.Add(-3 * RotateInterval)},
		{Endpoint: "2.2.2.2:51820", LastAttemptedAt: now.Add(-2 * RotateInterval)},
		{Endpoint: "3.3.3.3:51820", LastAttemptedAt: now.Add(-5 * RotateInterval)},
	}

	candidate, ok := nextCandidate(endpoints, now)
	if !ok {
		t.Fatal("expected a candidate")
	}
	if candidate != "3.3.3.3:51820" {
		t.Errorf("candidate = %q, want least-recently-attempted", candidate)
	}
}

func TestNextCandidate_FreshEndpointWinsOverAttempted(t *testing.T) {
	now := rotationBase
	endpoints := []PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", LastAttemptedAt: now.Add(-2 * RotateInterval)},
		{Endpoint: "2.2.2.2:51820"}, // just arrived from the server
	}

	candidate, ok := nextCandidate(endpoints, now)
	if !ok {
		t.Fatal("expected a candidate")
	}
	if candidate != "2.2.2.2:51820" {
		t.Errorf("candidate = %q, want the never-attempted endpoint", candidate)
	}
}

func TestNextCandidate_DwellExpiryAllowsNextRotation(t *testing.T) {
	now := rotationBase
	endpoints := []PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", LastAttemptedAt: now.Add(-RotateInterval)},
	}

	candidate, ok := nextCandidate(endpoints, now)
	if !ok {
		t.Fatal("expected a candidate once the dwell has passed")
	}
	if candidate != "1.1.1.1:51820" {
		t.Errorf("candidate = %q, want the sole endpoint again", candidate)
	}
}
