FROM ubuntu:bionic
MAINTAINER Gabe Fierro <gtfierro@eecs.berkeley.edu>

RUN apt-get update && apt-get install -y libssl-dev libssl1.1 && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

ADD pundat entrypoint.sh /bin/

# where the entity file goes
VOLUME /etc/giles

ENTRYPOINT [ "/bin/entrypoint.sh" ]
