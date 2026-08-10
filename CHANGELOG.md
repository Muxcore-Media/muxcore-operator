# Changelog

## [v0.1.0] — 2026-08-10

### Added
- `MuxCorePlatform` CRD (`muxcore.media/v1alpha1`) for minimal platform (muxcored + sidecars)
- controller-runtime reconciler creating Deployments + Services with owner references
- Sample CR mirroring `mvp/deploy` minimal platform module set
- Unit test with fake client
