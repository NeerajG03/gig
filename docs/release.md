# Release Process

## How to Release

```bash
git tag v0.0.3
git push origin v0.0.3
```

That's it. The [release workflow](../.github/workflows/release.yml) handles everything else.

## What Happens Automatically

1. **Tests run** — unit + E2E CLI tests must pass
2. **GitHub Release created** — with auto-generated release notes from commits
3. **Homebrew tap updated** — `Formula/gig.rb` in [homebrew-tap](https://github.com/NeerajG03/homebrew-tap) is updated with the new version and tarball SHA256

Users get the update via `brew upgrade gig`.

## Versioning

Using `0.x.y` during beta. The version is injected at build time via ldflags:

```
-X main.version=0.0.3
```

`gig --version` prints the injected version (defaults to `dev` for local builds).

## Secrets

| Secret | Purpose |
|--------|---------|
| `HOMEBREW_TAP_TOKEN` | Fine-grained PAT scoped to `NeerajG03/homebrew-tap` with Contents read/write. Used to push formula updates. |

## Manual Homebrew Install Verification

```bash
brew update
brew install neerajg03/tap/gig   # first install
brew upgrade gig                  # after a new release
gig --version                     # verify
```
