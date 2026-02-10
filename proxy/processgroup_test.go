package proxy

// This file previously contained tests for the old group-based swap semantics
// (TestProcessGroup_ProxyRequestSwapIsTrueParallel, TestProcessGroup_ProxyRequestSwapIsFalse,
// TestProcessGroup_DefaultHasCorrectModel, TestProcessGroup_HasMember).
//
// These tests are no longer relevant after the architectural change (commit 345e1f3)
// which transitioned from group-based model management to per-model process lanes.

