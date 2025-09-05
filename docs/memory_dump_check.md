## Vendor prompts memory-dump sanity check

Goal: Ensure plaintext vendor templates are not trivially found in a process memory dump.

Prereqs:
- gdb (for gcore)
- Encrypted vendor assets generated (use `make vendor-prompts-encrypt`) and build with `-tags vendor_prompts`.

Quick run (Linux):

1. Build and run the smoke harness and capture a core dump
   - The harness renders prompts in a loop for a few seconds.
   - The Makefile target automates: build, run, gcore, grep.

2. From repo root, run:

   make vendor-memdump-check

Expected output:
- A core file `core_render_smoke.<pid>` may be produced if gdb/gcore is available.
- The grep step should show no raw vendor template markers like `{{VAR:`. The final composed prompts may appear (that’s acceptable), but the original raw vendor template bodies should not be present in the binary nor trivially in the dump.

Notes:
- If no core file is produced, ensure `gdb`/`gcore` is installed and your environment permits core dumping.
- This is a sanity check, not a formal security proof. Keep keys rotating and avoid logging plaintext template data.
