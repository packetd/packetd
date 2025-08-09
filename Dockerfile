FROM centos:8

RUN yum update
RUN yum install -y libpcap libpcap-devel git golang

COPY . /packetd-project
WORKDIR /packetd-project

RUN make build
RUN cp /packetd-project/packetd /usr/local/bin/packetd

ENTRYPOINT ["packetd"]
