# CI failures

- When `test-lint-node-version-min-supported` in `.rwx/integration/lint-test.yml` fails, fix the reported errors, including outdated package versions, before proceeding. Keep `--warnings-as-errors` and the test assertions intact rather than weakening the test to make CI pass.
