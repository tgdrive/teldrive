# Installation

Teldrive can be installed directly on your system using the release installer, or you can run it from source while developing. If you prefer Docker, skip this section and proceed to the [Usage Guide](/docs/getting-started/usage.md).

## One-Line Installers

Choose the appropriate installation method for your operating system:

::: code-group
```sh [macOS/Linux (curl)]
curl -sSL instl.vercel.app/teldrive | bash
```

```powershell [PowerShell/cmd.exe]
powershell -c "irm https://instl.vercel.app/teldrive?platform=windows|iex"
```
:::

The installer will download the latest Teldrive binary and set it up on your system. Once installed, you can run Teldrive from any terminal window.

## Run From Source

If you are working from this repository:

```sh
task deps
task ui
task run
```

Use `task docs:dev` to preview the in-repo documentation site locally.

## Verifying Installation

After installation, verify that Teldrive is installed correctly:

```sh
teldrive version
```

This should display the current version of Teldrive.

## Next Steps

Now that you have installed Teldrive, proceed to the [Usage Guide](/docs/getting-started/usage.md) to configure and start the server.
