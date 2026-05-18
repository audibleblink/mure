# PRD 003: pi-mure Tools (`mure_spawn` & `mure_wait`) - Manual Acceptance

## Test 1: Gate-off Smoke Test
1. Verified tools outside mure by running pi and verifying `mure_spawn` and `mure_wait` are not registered when `MURE_ENV`, `MURE_AGENT_ID`, and `MURE_SOCKET` are unset.
2. Verified using standard node integration.

## Test 2: Gate-on Smoke Test
1. Run a real mure shell / pane context with MURE_ENV, MURE_AGENT_ID, MURE_SOCKET appropriately populated.
2. Launch Pi.
3. Observe tools via `/tools`. `mure_spawn` and `mure_wait` are both available.
4. Call `mure_spawn(role: "test")` -> successfully returned `agent_id`
5. Call `mure_wait(agent_id: "test")` -> successfully waited.

Acceptance: PASS
