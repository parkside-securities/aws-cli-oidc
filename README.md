# aws-cli-oidc

CLI tool for retrieving temporary AWS credentials using an OIDC provider.


## How does it work?

[AWS Identity Providers and Federation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers.html)
supports IdPs that are compatible with [OpenID Connect (OIDC)](http://openid.net/connect/). This tool works as an OIDC
client. If the federation between the AWS account and the IdP is established, and an OIDC client for this tool is
registered in the IdP, you can get AWS temporary credentials via standard browser login. It means you don't need to pass
your credential of the IdP to this tool.

Please refer to the following diagrams on how it works.
Steps (1) and (2) are slightly simplified as there is more going on but it should give an overview.

```
                (1) authenticate user [username, password]        +---------------+
    +------------------------------------------------------------>|               |
    |                                                             | OIDC Provider |
    |      +------------------------------------------------------|               |
    |      |       (2) authentication successful [id_token]       +---------------+
    |      |                                                                |
    |      v                                                                |
+--------------+                                                            |
|              |                                        trust OIDC provider |
| aws-cli-oidc |                                                            |
|              |                                                            |
+--------------+                     AWS                                    |
    ^      |                       +----------------------------------------|-----+
    |      | (3) assume role A     |  +---------+       +--------+--------------+ |
    |      |     [id_token]        |  |   STS   |      -| Role A | Trust Policy | |
    |      +------------------------->|         |    -/ +--------+--------------+ |
    |                              |  |         | --/             .               |
    |                              |  |         |/                .               |
    |                              |  |         |                 .               |
    +---------------------------------|         |       +--------+--------------+ |
     (4) temporary AWS credential  |  |         |       | Role Z | Trust Policy | |
         [aws_key, aws_secret]     |  +---------+       +--------+--------------+ |
                                   +----------------------------------------------+
```

## Prerequisite AWS and OIDC provider settings before using this tool

Before using this tool, the system administrator need to setup the following configuration.

- Identity Federation using OIDC between AWS and the OIDC provider.
  See https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers.html
- Registration of an OIDC/OAuth2 client for this CLI tool in the OIDC provider.
  Note: The OIDC provider must allow `http://localhost:52327` to be specified as a redirect URL.

## Tested OIDC Provider

- [Google account](https://accounts.google.com/.well-known/openid-configuration)
- Cognito User Pool

## Install

### Homebrew

If you are on a Mac, you can use [Homebrew](https://brew.sh) to install the tool:
```
brew install mxie1563/mbr/aws-cli-oidc
```
or
```
brew tap mxie1563/mbr
brew install aws-cli-oidc
```

### Binary Releases
You can download binary releases for all major operating system from the [Releases page](https://github.com/mbrtargeting/aws-cli-oidc/releases).

### Build From Source
Alternatively, you can also build from source.
```shell script
make build
```
After building, the binaries will reside in the `bin/` subfolder.

## Usage

```
aws-cli-oidc.

Usage:
  aws-cli-oidc get-cred <idp> <role> [print] [<seconds>]
  aws-cli-oidc cache (show [token]| clear)
  aws-cli-oidc setup <idp>
  aws-cli-oidc -h | --help

Options:
  -h --help  Show this screen.
```

### Setup

Before you use tool you need to setup an identity provider first.
There are two options to do so.

The first one is to provide a YAML file containing the configuration.
An example configuration with an identity provider named "google" might look like this:
```
google:
  oidc_server: accounts.google.com
  auth_url: https://accounts.google.com/o/oauth2/v2/auth
  token_url: https://oauth2.googleapis.com/token
  client_id: my_client_id
  client_secret: my_client_secret
  aws_region: us-east-1
  max_session_duration_seconds: 3600
```
This file must be saved as `$AWS_CLI_OIDC_CONFIG/config.yaml` where `AWS_CLI_OIDC_CONFIG` is an environment variable
pointing to the root config folder.
If `AWS_CLI_OIDC_CONFIG` is not set it defaults to `~/.aws-cli-oidc/`.

The alternative to writing a config file by hand is to use the guided setup via `aws-cli-oidc setup <idp>`
where `<idp>` is the name you wish to give to this configuration (like "google" in the above example).
After finishing this guided survey, the tool will append the resulting provider configuration to the
config file.

When you are done with the configuration, you can reference the providers `aws-cli-oidc get-cred <idp> <role>` using
the short name you gave them (`aws-cli-oidc get-cred google <role>` for the above example).

### Get temporary AWS credentials

To obtain temporary AWS credential, execute the `aws-cli-oidc get-cred <idp> <role>` command where `<idp>` is the name
of a configured identity provider and `<role>` is the role you want to assume on a AWS account
(for example, `aws-cli-oidc get-cred google arn:aws:iam::123443211234:role/my-role`).
If you did not log in for a long time or if you are using the tool for the first time, it opens your browser for you to authenticate.
If the authentication is successful, AWS temporary credentials will be output in the JSON format.

You can also use this tool directly as a credential process.
For this, add the following lines to your `.aws/credentials` file.
```
[my-profile]
credential_process=aws-cli-oidc get-cred google arn:aws:iam::123443211234:role/my-role
```
And make sure that the `aws-cli-oidc` is on your `PATH` or, alternatively, provide the full path to the binary in the
configuration above.


## Licence

Licensed under the [MIT](/LICENSE) license.


## Authors

- [Hiroyuki Wada](https://github.com/wadahiro)
- [Ströer SSP GmbH](https://www.stroeer.de/konvergenz-konzepte/daten-technologien/stroeer-ssp.html)
- [Michael Xie](https://github.com/mxie1563)
