---
title: Conduit
description: Secure, self-hosted observability for AI agents.
---

<section class="hero">
  <img src="{{ '/assets/img/logo.svg' | relative_url }}" alt="Conduit logo">
  <h1>Observability for AI agents,<br>inside your own walls.</h1>
  <p class="lead">Conduit traces every agent step, tool call and LLM call (status, errors, tokens and cost) and keeps the data in your infrastructure. It's built in Go, with no npm, no JVM and no supply-chain surprises.</p>
  <div class="cta">
    <a class="button primary" href="{{ '/deployment.html' | relative_url }}">Get started</a>
    <a class="button secondary" href="{{ '/roadmap.html' | relative_url }}">See the roadmap</a>
  </div>
</section>

<section class="section">
  <h2>Built for teams that take security seriously</h2>
  <p class="sub">Everything you need to see what your agents are doing, and nothing you'd have to explain to a security review.</p>
  <div class="features">
    <div class="feature"><h3>Identity you can trust</h3><p>Agents authenticate with mTLS client certificates. The certificate's CommonName is the agent's identity, and attributes can't spoof it.</p></div>
    <div class="feature"><h3>Hardened ingestion</h3><p>Every span is validated before it reaches storage. Rate limits are charged per telemetry item, so batching can't get around them.</p></div>
    <div class="feature"><h3>LLM-aware traces</h3><p>Model, tokens, cost and errors per span, rolled up per trace. Turn off prompt storage with one switch.</p></div>
    <div class="feature"><h3>A UI that's quick to read</h3><p>One row per trace, filters, stable paging, and a span tree with a waterfall timeline. No JavaScript framework.</p></div>
    <div class="feature"><h3>Small and auditable</h3><p>Static Go binaries in <code>FROM scratch</code> images, a strict Content-Security-Policy, and few dependencies.</p></div>
    <div class="feature"><h3>Production basics included</h3><p>Login or GCP IAP sign-in, health probes, configurable retention, Kubernetes manifests, and an end-to-end CI smoke test.</p></div>
  </div>
</section>

<section class="band">
  <div class="section">
    <h2>Documentation</h2>
    <p class="sub">Start with the architecture, or go straight to deploying and instrumenting your first agent.</p>
    <div class="doc-links">
      <a href="{{ '/architecture.html' | relative_url }}"><strong>Architecture</strong><span>Components, data flow, delivery guarantees and the storage schema.</span></a>
      <a href="{{ '/deployment.html' | relative_url }}"><strong>Deployment</strong><span>Docker Compose, Kubernetes, health checks and configuration.</span></a>
      <a href="{{ '/security.html' | relative_url }}"><strong>Security</strong><span>The threat model, mTLS identity, UI sign-in and hardening.</span></a>
      <a href="{{ '/sdk-go.html' | relative_url }}"><strong>Go SDK</strong><span>Instrument a Go agent in a few lines.</span></a>
      <a href="{{ '/sdk-python.html' | relative_url }}"><strong>Python SDK</strong><span>Context managers that are safe across threads and asyncio.</span></a>
      <a href="{{ '/roadmap.html' | relative_url }}"><strong>Roadmap</strong><span>What's shipped, what's next and the open decisions.</span></a>
    </div>
  </div>
</section>
