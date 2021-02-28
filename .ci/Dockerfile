FROM docker.internal.kevinlin.info/infra/ci-base:0.3.1

ENV APINDEX_VERSION e8ed53a76dfd2dfaf2aa444f666b4513d3108653

# Release dependencies
ADD https://storage.kevinlin.info/deploy/external/apindex/$APINDEX_VERSION/release.tar.gz apindex.tar.gz
RUN sudo tar xvf apindex.tar.gz
RUN sudo mv bin/* /usr/local/bin/
RUN sudo mv share/* /usr/local/share/
COPY resources/static/header.template.html /usr/local/share/apindex/header.template.html
COPY resources/static/footer.template.html /usr/local/share/apindex/footer.template.html
