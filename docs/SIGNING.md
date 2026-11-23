# Signing releases

i-mux's release can sign its programs on Windows and macOS (ARCHITECTURE.md D100). Signing is
off until the repository has a certificate: the release then builds unsigned, as before. Linux
needs no signing; the installers check every download against `checksums.txt`.

| System | What is signed | Tool (on the Linux release runner) | Turned on by |
|---|---|---|---|
| Windows | `imux.exe`, `imux-core.exe`, `setup.exe` and its uninstaller | [jsign](https://ebourg.github.io/jsign/) 7.6, Authenticode | the variable `IMUX_SIGN_STORETYPE` |
| macOS | `imux`, `imux-core` (signed and notarized) | GoReleaser's `notarize.macos` (quill) | the secret `MACOS_SIGN_P12` |

## Windows

Since 2023 a code-signing key must stay in hardware or a cloud HSM, so the release signs through
a signing service rather than with a key file. Any of these works with jsign:

| Option | Cost | Publisher shown | Store type |
|---|---|---|---|
| [SignPath Foundation](https://signpath.org) | free for open-source projects, after review | SignPath Foundation | `SIGNPATH` |
| [Azure Trusted Signing](https://learn.microsoft.com/azure/trusted-signing/) | about $10 a month; identity validation | you or your company | `TRUSTEDSIGNING` |
| Certum, SSL.com, DigiCert … certificates with a cloud HSM | $25–500 a year | you or your company | `ESIGNER`, `DIGICERTONE`, `AZUREKEYVAULT` … |

Set these in GitHub → Settings → Secrets and variables → Actions:

| Name | Kind | Value |
|---|---|---|
| `IMUX_SIGN_STORETYPE` | variable | jsign's `--storetype`, as in the table above |
| `IMUX_SIGN_KEYSTORE` | variable | jsign's `--keystore` (Trusted Signing: the endpoint, such as `weu.codesigning.azure.net`; SignPath: the organization id) |
| `IMUX_SIGN_ALIAS` | variable | jsign's `--alias` (Trusted Signing: `<account>/<profile>`; SignPath: `<project>/<signing policy>`) |
| `IMUX_SIGN_STOREPASS` | secret | jsign's `--storepass` (Trusted Signing: `<tenant>\|<client>\|<secret>`; SignPath: an API token) |
| `IMUX_SIGN_TSA` | variable, optional | the timestamp server (default `http://timestamp.digicert.com`) |

The next tag's release then signs every Windows file before it is archived and checksummed.
Check a release on Windows:

```powershell
Get-AuthenticodeSignature "$env:LOCALAPPDATA\Programs\i-mux\imux.exe" | Format-List Status, SignerCertificate
```

`Status` should be `Valid`. SmartScreen may still warn for a while: Windows trusts a new
publisher as its programs are downloaded. Company policies (AppLocker, WDAC) may also need the
publisher allowed by IT; a signed program is what such a rule can name.

CI tests the whole path at every push with a self-signed certificate it makes on the spot, so a
broken signing step shows before a real release needs it.

## macOS

1. Join the [Apple Developer Program](https://developer.apple.com/programs/) ($99 a year).
2. Create a **Developer ID Application** certificate, export it with its key from Keychain
   Access as a `.p12` file with a password, and encode it: `base64 -i cert.p12 | pbcopy`.
3. Create an App Store Connect API key (Users and Access → Integrations → Keys) and download its
   `.p8` file.
4. Set these secrets:

| Name | Value |
|---|---|
| `MACOS_SIGN_P12` | the base64 `.p12` |
| `MACOS_SIGN_PASSWORD` | its password |
| `MACOS_NOTARY_ISSUER_ID` | the key's issuer id |
| `MACOS_NOTARY_KEY_ID` | the key's id |
| `MACOS_NOTARY_KEY` | the `.p8` file's contents |

The next release signs and notarizes `imux` and `imux-core`. Check on a Mac:

```sh
codesign -dv --verbose=2 ~/.local/bin/imux
spctl -a -vv -t install ~/.local/bin/imux
```
