FROM mcr.microsoft.com/devcontainers/go:1

# install the devcontainers CLI and add it to the PATH
RUN curl -fsSL https://raw.githubusercontent.com/devcontainers/cli/main/scripts/install.sh | sh -s -- --prefix /opt/devcontainers
RUN ln -s /opt/devcontainers/bin/devcontainer /usr/local/bin
