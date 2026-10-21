# nebu's task runner is the justfile. This Makefile is a compatibility
# shim kept for one release so existing muscle memory and any external
# scripts keep working: `make test` forwards to `just test`.
#
# Do not add task logic here — it goes in the justfile (BUILD-1 of the
# OBSRVR Engineering Standard). This file is scheduled for removal one
# release after the one that introduced it; see CHANGELOG.md.
#
# The shim preserves success and failure but NOT the exact exit code:
# make reports its own 2 for any recipe failure, where just would have
# exited 1. Anything that branches on a specific exit code must call
# just directly.

JUST := $(shell command -v just 2>/dev/null)

# Forward any target to just. .DEFAULT handles every name that is not
# defined below, so the shim needs no per-recipe maintenance.
.PHONY: default
default:
	@$(MAKE) --no-print-directory forward TARGET=

.DEFAULT:
	@$(MAKE) --no-print-directory forward TARGET=$@

.PHONY: forward
forward:
	@if [ -z "$(JUST)" ]; then \
		echo "ERROR: 'just' is not installed."; \
		echo ""; \
		echo "nebu's task runner is the justfile. Get it with:"; \
		echo "  direnv allow      # or: nix develop"; \
		echo ""; \
		echo "The dev shell provides it. See CONTRIBUTING.md."; \
		exit 1; \
	fi; \
	if [ -n "$(TARGET)" ]; then suggestion="just $(TARGET)"; else suggestion="just"; fi; \
	echo "note: 'make' is a shim for 'just' and will be removed; run '$$suggestion' instead." >&2; \
	exec $(JUST) $(TARGET)
