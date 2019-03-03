FROM golang:1.12-stretch

ENV HOME /home/jenkins

# User configuration for proper interopability with the CI container's environment
RUN groupadd -g 10000 jenkins
RUN useradd -c "Jenkins user" -d $HOME -u 10000 -g 10000 -m jenkins

# APT dependencies
RUN apt-get update && apt-get install -y \
    cmake \
    python3

# Build dependencies
RUN go get -u -v golang.org/x/lint/golint
RUN go get -u -v golang.org/x/tools/cmd/stringer

# Release dependencies
RUN wget https://github.com/libthinkpad/apindex/archive/2.2.tar.gz -O apindex.tar.gz
RUN tar xvf apindex.tar.gz
RUN cd apindex-2.2/
RUN cd apindex-2.2/ && cmake . -DCMAKE_INSTALL_PREFIX=/usr
RUN make -C apindex-2.2/ install
RUN echo '<a style="font-size: 12px" href="https://github.com/LINKIWI/dotproxy">dotproxy</a>' > /usr/share/apindex/footer.template.html

# Prepare environment for non-root user
RUN chown -R jenkins:jenkins $HOME $GOPATH
