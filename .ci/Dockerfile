FROM docker.internal.kevinlin.info/infra/ci-base:0.2.2

ENV APINDEX_VERSION 81e3494bcbc3207f8669f3be1d7446c9e134a2a0

# Build dependencies
RUN go get -u -v golang.org/x/lint/golint
RUN go get -u -v golang.org/x/tools/cmd/stringer

# Release dependencies
ADD https://storage.kevinlin.info/deploy/external/apindex/$APINDEX_VERSION/release.tar.gz apindex.tar.gz
RUN sudo tar xvf apindex.tar.gz
RUN sudo mv bin/* /usr/local/bin/
RUN sudo mv share/* /usr/local/share/
COPY resources/static/header.template.html /usr/local/share/apindex/header.template.html
COPY resources/static/footer.template.html /usr/local/share/apindex/footer.template.html
