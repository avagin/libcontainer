FROM crosbymichael/golang

RUN apt-get update && apt-get install -y gcc make \
    build-essential \
    pkg-config \
    libtool \
    autoconf \
    git-core \
    bison \
    flex \
    libselinux1-dev \
    libapparmor-dev

RUN go get code.google.com/p/go.tools/cmd/cover

ENV GOPATH $GOPATH:/go/src/github.com/docker/libcontainer/vendor
RUN go get github.com/docker/docker/pkg/term

RUN git clone https://github.com/avagin/libct.git /go/src/github.com/avagin/libct && \
    cd /go/src/github.com/avagin/libct && \
    git submodule update --init && \
    cd .shipped/libnl && \
    ./autogen.sh && \
    ./configure && make -j $(nproc) && \
    cd ../../ && \
    make clean && make -j $(nproc)

ENV LIBRARY_PATH $LIBRARY_PATH:/go/src/github.com/avagin/libct/src:/go/src/github.com/avagin/libct/.shipped/libnl/lib/.libs/

# setup a playground for us to spawn containers in
RUN mkdir /busybox && \
    curl -sSL 'https://github.com/jpetazzo/docker-busybox/raw/buildroot-2014.02/rootfs.tar' | tar -xC /busybox

RUN curl -sSL https://raw.githubusercontent.com/docker/docker/master/hack/dind -o /dind && \
    chmod +x /dind

COPY . /go/src/github.com/docker/libcontainer
WORKDIR /go/src/github.com/docker/libcontainer
RUN cp sample_configs/minimal.json /busybox/container.json

RUN go get -d -v ./...
RUN make direct-install

ENTRYPOINT ["/dind"]
CMD ["make", "direct-test"]
