# Build assets (Wails v3)

Platform packaging is driven by the Taskfiles in this directory and the root
`Taskfile.yml`. Prefer:

```bash
cd cmd/informer-ui
wails3 build                          # binary → bin/
wails3 package                        # platform package
wails3 task darwin:package:universal  # macOS universal .app
```

`config.yml` holds product metadata. `nfpm.yaml` and `package/` remain the
Linux `.deb` sources used by the release workflow.
