FROM centos:7

RUN yum clean all && rm -f /var/lib/rpm/__db* && rpm --rebuilddb
RUN curl -o /etc/yum.repos.d/CentOS-Base.repo http://mirrors.aliyun.com/repo/Centos-7.repo
RUN yum install -y libpcap libpcap-devel git wget make gcc iproute

ARG GO_VERSION=1.24.0

RUN cd /tmp && wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && rm -rf /usr/local/go && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

COPY . /packetd-project

RUN make build
RUN cp /packetd-project/packetd /usr/local/bin/packetd
RUN rm -rf /packetd-project

WORKDIR /

ENTRYPOINT ["packetd"]
