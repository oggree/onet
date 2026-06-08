# Onet Project TODO & Future Improvements

## 1. Fix CGO Cross-Compilation for macOS (Darwin) Targets

- **Status**: ❌ Failed
- **Error Logs**:
  ```text
  ⨯ release failed after 2m41s                      
    error=
    │ build failed: exit status 1: # github.com/mattn/go-sqlite3
    │ error: unable to create compilation: AccessDenied
    target=darwin_arm64_v8.0
  ```
- **Context**:
  To support future migration to `cr-sqlite` (which compiles as a C extension), we must keep CGO enabled (`CGO_ENABLED=1`). However, during the cross-compilation step using `zig cc` in GitHub Actions on an Ubuntu runner, `zig` fails with `AccessDenied` when attempting to write intermediate compilation assets for target `darwin_arm64`.
- **Potential Fixes**:
  1. **Configure Temp/Cache Permissions**: Ensure the `setup-zig` action has appropriate permissions or explicitly configure Zig cache directories (`ZIG_GLOBAL_CACHE_DIR` or `ZIG_LOCAL_CACHE_DIR`) to point to writable, workspace-specific paths.
  2. **Multi-Platform Runners (Recommended)**: Split the release job in GitHub Actions into a matrix. Run Linux builds on `ubuntu-latest` and Darwin/macOS builds natively on `macos-latest` to avoid cross-compilation issues entirely.

---

## 2. Server Security Tasks

- [ ] **Rotate Default API Key**: Change `api_key: default-key-change-me` in `/etc/onet.yaml` on your server. This secures the newly added HTTP Basic Authentication on the Arc setup wizard.
