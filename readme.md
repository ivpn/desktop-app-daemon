_This is a legacy project.
Development has been moved to a new repository: https://github.com/ivpn/desktop-app_

# IVPN Daemon (Windows/macOS/Linux)

**IVPN Daemon** is a core module of IVPN Client software for desktop platforms (Windows/macOS/Linux) built mostly using Go language.  
It runs under privileged user as a system service/daemon.  

Some of the features include:  
  - multiple protocols (OpenVPN, WireGuard)  
  - Kill-switch  
  - custom DNS  
  - Multi-Hop  
  - AntiTracker  

This project is in use by [IVPN Client UI](https://github.com/ivpn/desktop-app-ui2) and [IVPN CLI](https://github.com/ivpn/desktop-app-cli) projects.

IVPN Client app is distributed on the official site [www.ivpn.net](https://www.ivpn.net).  

* [About this Repo](#about-repo)
* [Installation](#installation)
* [Versioning](#versioning)
* [Contributing](#contributing)
* [Security Policy](#security)
* [License](#license)
* [Authors](#Authors)
* [Acknowledgements](#acknowledgements)

<a name="about-repo"></a>
## About this Repo

This is the official Git repo of the [IVPN Daemon](https://github.com/ivpn/desktop-app-daemon).


<a name="installation"></a>
## Installation

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Requirements

#### Windows

  - Build Tools for Visual Studio 2019 ('Windows SDK 10.0', 'MSVC v142 C++ x64 build tools', 'C++ ATL for latest v142 build tools')
  - gcc compiler e.g. [TDM GCC](https://jmeubank.github.io/tdm-gcc/download/)
  - Go 1.13+
  - Git

#### macOS

  - Xcode Command Line Tools
  - Go 1.13+
  - Git

#### Linux
  - Go 1.13+
  - packages: 'rpm' and 'libiw-dev'
  - Git

### Compilation

#### Windows  

**Note!**
  IVPN Daemon must be installed appropriately on a target system.  
  We recommend using [IVPN Client UI](https://github.com/ivpn/desktop-app-ui2) project to build a Windows installer for IVPN software.  

  To compile IVPN service binary run the batch file from the terminal.  
  Use Developer Command Prompt for Visual Studio (required for building native sub-projects).  

```
git clone https://github.com/ivpn/desktop-app-daemon.git
cd desktop-app-daemon
References/Windows/scripts/build-all.bat <VERSION_X.X.X> exclude32bit
```

#### macOS  

  **Note!**
  IVPN Daemon must be installed appropriately on a target system.  
  We recommend using [IVPN Client UI](https://github.com/ivpn/desktop-app-ui2) project to build a macOS DMG package for IVPN software.  

```
git clone https://github.com/ivpn/desktop-app-daemon.git
cd desktop-app-daemon
References/macOS/scripts/build-all.sh -v <VERSION_X.X.X>
```

  The batch script will compile IVPN Service binary and all required dependencies (OpenVPN, WireGuard).
  Compiled binaries location:

  - WireGuard:  `References/macOS/_deps/wg_inst`
  - OpenVPN:  `References/macOS/_deps/openvpn_inst/bin`
  - IVPN Service: `IVPN Agent`

  **Note!**
  In order to run application as macOS daemon, the binary must be signed by Apple Developer ID.

#### Linux  

**Note!**
  IVPN Daemon must be installed appropriately on a target system.  
  We recommend using:  
    - [IVPN CLI](https://github.com/ivpn/desktop-app-cli) project to build a 'base' Linux redistributable packages of IVPN software  
    - [IVPN Client UI](https://github.com/ivpn/desktop-app-ui2) project to build a 'UI' Linux redistributable packages of IVPN software  

```
git clone https://github.com/ivpn/desktop-app-daemon.git
cd desktop-app-daemon
./References/Linux/scripts/build-all.sh -v <VERSION_X.X.X>
```

  The compiled binary can be found at `References/Linux/scripts/_out_bin`

<a name="versioning"></a>
## Versioning

Project is using [Semantic Versioning (SemVer)](https://semver.org) for creating release versions.

SemVer is a 3-component system in the format of `x.y.z` where:

`x` stands for a **major** version  
`y` stands for a **minor** version  
`z` stands for a **patch**

So we have: `Major.Minor.Patch`

<a name="contributing"></a>
## Contributing

If you are interested in contributing to IVPN Daemon for IVPN Client Desktop project, please read our [Contributing Guidelines](/.github/CONTRIBUTING.md).

<a name="security"></a>
## Security Policy

If you want to report a security problem, please read our [Security Policy](/.github/SECURITY.md).

<a name="license"></a>
## License

This project is licensed under the GPLv3 - see the [License](/LICENSE.md) file for details.

<a name="Authors"></a>
## Authors

See the [Authors](/AUTHORS) file for the list of contributors who participated in this project.

<a name="acknowledgements"></a>
## Acknowledgements

See the [Acknowledgements](/ACKNOWLEDGEMENTS.md) file for the list of third party libraries used in this project.
