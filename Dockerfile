FROM centos:7

ADD bin/local-storage-udev-writer /usr/local/bin
ENTRYPOINT ["/usr/local/bin/local-storage-udev-writer"]
