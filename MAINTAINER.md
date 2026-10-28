# Maintaining session-menu

## Making a release

Push a tag `v<major>.<minor>.<patch>`. The Release workflow (`.github/workflows/release.yml`) then
tests the code and builds the archives with `package.sh`, and publishes them as a GitHub release.
The programs print that version with `-version`. Run by hand from the Actions tab, the workflow
builds the archives without publishing a release, and keeps them as an artifact of the run.

To build the archives locally, into `dist/`:

```
./package.sh
```
