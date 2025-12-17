# LinuxPkg Plugin for Relicta

Official LinuxPkg plugin for [Relicta](https://github.com/relicta-tech/relicta) - Build deb/rpm packages for Linux.

## Installation

```bash
relicta plugin install linuxpkg
relicta plugin enable linuxpkg
```

## Configuration

Add to your `release.config.yaml`:

```yaml
plugins:
  - name: linuxpkg
    enabled: true
    config:
      # Add configuration options here
```

## License

MIT License - see [LICENSE](LICENSE) for details.
