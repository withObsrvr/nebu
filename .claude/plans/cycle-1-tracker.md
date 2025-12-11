# Cycle 1: Progress Tracker

**Shape:** Core Runtime & RPC Source
**Appetite:** 1 week (5 days)
**Started:** _____
**Deadline:** _____

---

## Hill Chart

Track your position daily:

```
Day 1: [ ]━━━━━━━━━━━━━━━━┊━━━━━━━━━━━━━━━━[ ]  Target: 25%
Day 2: [ ]━━━━━━━━━━━━━━━━┊━━━━━━━━━━━━━━━━[ ]  Target: 50% ⭐
Day 3: [ ]━━━━━━━━━━━━━━━━┊━━━━━━━━━━━━━━━━[ ]  Target: 75%
Day 4: [ ]━━━━━━━━━━━━━━━━┊━━━━━━━━━━━━━━━━[ ]  Target: 90%
Day 5: [ ]━━━━━━━━━━━━━━━━┊━━━━━━━━━━━━━━━━[ ]  Target: 100% 🚢

       Figuring it out    ┊    Making it happen
```

Mark your position with an X each day.

---

## Daily Log

### Day 1: Foundation & RPC Connection
**Date:** _____
**Hill Position:** _____%

#### Completed:
- [ ] Project scaffolding
- [ ] `LedgerSource` interface
- [ ] `RPCLedgerSource` implementation
- [ ] First integration test

#### Blockers:
-

#### Tomorrow:
-

---

### Day 2: Processor Interfaces & Runtime
**Date:** _____
**Hill Position:** _____%

#### Completed:
- [ ] Processor types defined
- [ ] Origin interface
- [ ] Emitter helper (or SKIPPED)
- [ ] Runtime.RunOrigin implemented

#### Blockers:
-

#### At top of hill? YES / NO
If NO, what's still unclear? _____

#### Tomorrow:
-

---

### Day 3: Examples & Integration
**Date:** _____
**Hill Position:** _____%

#### Completed:
- [ ] Counting origin example
- [ ] Event-emitting example (or SKIPPED)
- [ ] Cancellation tests

#### Scope cuts made:
-

#### Tomorrow:
-

---

### Day 4: Polish & Documentation
**Date:** _____
**Hill Position:** _____%

#### Completed:
- [ ] Error handling improved
- [ ] Logging added (or SKIPPED)
- [ ] Documentation written

#### Tomorrow:
-

---

### Day 5: Testing & Ship
**Date:** _____
**Hill Position:** _____%

#### Completed:
- [ ] Extended integration tests
- [ ] README written
- [ ] Final cleanup
- [ ] Tagged release: v0.1.0-alpha.1

#### SHIPPED: YES / NO

---

## Scope Cut Log

| Item | Reason | Day Cut |
|------|--------|---------|
|      |        |         |

---

## Decisions Made

| Decision | Rationale | Impact |
|----------|-----------|--------|
|          |           |        |

---

## Learnings for Next Cycle

What worked well:
-

What was harder than expected:
-

What to change:
-

Questions to resolve:
-

---

## Quick Reference

**Run tests:**
```bash
go test ./...
```

**Run example:**
```bash
go run examples/simple_origin/main.go
```

**Check formatting:**
```bash
go fmt ./...
go vet ./...
```

**Current focus:** _____________________
