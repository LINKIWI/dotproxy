FROM golang:1.12-stretch

ENV HOME /home/jenkins

# User configuration for proper interopability with the CI container's environment
RUN groupadd -g 10000 jenkins
RUN useradd -c "Jenkins user" -d $HOME -u 10000 -g 10000 -m jenkins

# APT dependencies
RUN apt-get update && apt-get install -y cmake

# Build dependencies
RUN go get -u -v golang.org/x/lint/golint
RUN go get -u -v golang.org/x/tools/cmd/stringer

# Release dependencies
RUN git clone https://github.com/LINKIWI/apindex.git && \
    git -C apindex/ reset --hard e3f64a1c9a6589b32324178dc216debf41d13d57
RUN cd apindex/ && cmake . -DCMAKE_INSTALL_PREFIX=/usr
RUN make -C apindex/ install
COPY resources/static/header.template.html /usr/share/apindex/header.template.html
COPY resources/static/footer.template.html /usr/share/apindex/footer.template.html

# Prepare environment for non-root user
RUN chown -R jenkins:jenkins $HOME $GOPATH
