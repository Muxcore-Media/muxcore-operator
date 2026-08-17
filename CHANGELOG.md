# Changelog

## [v0.1.0] — 2026-08-10

### Added
- `MuxCorePlatform` CRD (`muxcore.media/v1alpha1`) for minimal platform (muxcored + sidecars)
- controller-runtime reconciler creating Deployments + Services with owner references
- Sample CR mirroring `mvp/deploy` minimal platform module set
- Kind/k3d local-image samples under `config/samples/kind/` (`localhost:5000` / cluster-loaded tags)
- README day-1 preference: installer/compose over operator
- Unit test with fake client
