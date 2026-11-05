package main

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// The Get started page (/start) walks a new user from a running stack to
// their own agent's first trace. It never handles keys: certificates are
// issued on the command line, and the page only shows the commands.
const (
	onboardingCookie   = "conduit_onboarding"
	onboardingRecent   = 15 * time.Minute    // "telemetry is flowing"
	onboardingLookback = 30 * 24 * time.Hour // how far back a first trace counts
	onboardingRecheck  = 5 * time.Second     // cache for the not-yet-onboarded answer
)

// onboardingState caches whether a real (non-demo) agent has sent a trace,
// so pages can decide whether to offer Get started without a query each time.
// Once true it stays true for the life of the process.
type onboardingState struct {
	mu        sync.Mutex
	done      bool
	checkedAt time.Time
}

// onboarded reports whether a non-demo agent has sent a trace.
func (h *handlers) onboarded(ctx context.Context) bool {
	s := &h.onboarding
	s.mu.Lock()
	if s.done || time.Since(s.checkedAt) < onboardingRecheck {
		done := s.done
		s.mu.Unlock()
		return done
	}
	s.mu.Unlock()

	o, err := h.ch.QueryOnboarding(ctx, h.demoAgent, onboardingRecent, onboardingLookback)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkedAt = time.Now()
	if err != nil {
		h.logger.Warn("query onboarding", "err", err)
		return s.done
	}
	if o.LatestTraceID != "" {
		s.done = true
	}
	return s.done
}

func dismissedOnboarding(r *http.Request) bool {
	c, err := r.Cookie(onboardingCookie)
	return err == nil && c.Value == "dismissed"
}

// showStart: offer Get started in the nav (and land on it) until the user's
// own agent has sent a trace or they've dismissed it.
func (h *handlers) showStart(r *http.Request) bool {
	return !dismissedOnboarding(r) && !h.onboarded(r.Context())
}

func (h *handlers) onboardingData(r *http.Request) map[string]any {
	ctx := r.Context()
	data := map[string]any{
		"Page":        "start",
		"GatewayAddr": h.gatewayAddr,
		"DemoAgent":   h.demoAgent,
		"Ready":       h.ch.Ready(ctx) == nil,
	}
	o, err := h.ch.QueryOnboarding(ctx, h.demoAgent, onboardingRecent, onboardingLookback)
	if err != nil {
		h.logger.Warn("query onboarding", "err", err)
		data["QueryFailed"] = true
		return data
	}
	data["Status"] = o
	data["Connected"] = o.LatestTraceID != ""
	if o.LatestTraceID != "" {
		// Seen here first: update the cache so / and the nav follow at once.
		h.onboarding.mu.Lock()
		h.onboarding.done = true
		h.onboarding.mu.Unlock()
	}
	return data
}

func (h *handlers) start(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h.render(w, r, "start", h.onboardingData(r))
}

// startStatus is the htmx fragment that polls for the first trace. Once it
// has arrived, the fragment no longer carries hx-trigger, so polling stops.
func (h *handlers) startStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h.render(w, r, "start-status", h.onboardingData(r))
}

func (h *handlers) dismissStart(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     onboardingCookie,
		Value:    "dismissed",
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, defaultLandingPath, http.StatusSeeOther)
}
