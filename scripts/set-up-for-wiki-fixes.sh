#!/bin/bash
set -euo pipefail

find src -type f -print | perl -pe 's/^/- [ ] /' > Notes/_file-checklist.md
find tests -type f -print | perl -pe 's/^/- [ ] /' >> Notes/_file-checklist.md
find e2e-tests -type f -print | perl -pe 's/^/- [ ] /' >> Notes/_file-checklist.md
