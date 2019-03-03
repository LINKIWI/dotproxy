FROM golang:1.12-stretch

ENV HOME /home/jenkins

# User configuration for proper interopability with the CI container's environment
RUN groupadd -g 10000 jenkins
RUN useradd -c "Jenkins user" -d $HOME -u 10000 -g 10000 -m jenkins

# Build dependencies
RUN go get -u -v golang.org/x/lint/golint
RUN go get -u -v golang.org/x/tools/cmd/stringer

# Prepare environment for non-root user
RUN chown -R jenkins:jenkins $HOME $GOPATH
