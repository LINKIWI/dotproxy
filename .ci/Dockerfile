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
    git -C apindex/ reset --hard daf5a3f0ef11c337b5edde684ceb0edd0f66b14b
RUN cd apindex/ && cmake . -DCMAKE_INSTALL_PREFIX=/usr
RUN make -C apindex/ install
RUN echo '<a style="font-size: 12px" href="https://github.com/LINKIWI/dotproxy">dotproxy</a>' > /usr/share/apindex/footer.template.html

# Prepare environment for non-root user
RUN chown -R jenkins:jenkins $HOME $GOPATH
