# PrintWatcher release policy

- Every completed logical change must receive a new app version and matching annotated Git tag. Include `VERSION` in the same commit as the change. Do not reuse or move an existing release tag.
- Use exactly `vYYYY.MM.DD.I`. Use the user's date in `America/Denver`, zero-pad month/day, and start the daily increment at 1. Check both local and origin tags and `VERSION`; choose one greater than the highest increment for that date.
- Keep the leading `v` in `VERSION`, the Git tag, Docker image tag, and app display. Do not prepend a second `v` in the UI.
- For work authorized for publication, push the change commit and its matching tag. The tag-triggered Build and Publish workflow publishes GHCR `latest`, the exact version tag, and a GitHub release. Verify tests, the workflow result, and the published image revision/version before reporting completion.
- `main` pushes run CI without publishing a container. Avoid publishing an intermediate change under an old version.
