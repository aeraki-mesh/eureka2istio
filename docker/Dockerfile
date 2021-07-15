FROM registry.access.redhat.com/ubi8/ubi-init


ENV EK_ADDR=""

COPY eureka2istio /usr/local/bin/
ENTRYPOINT /usr/local/bin/eureka2istio -ekaddr=$EK_ADDR
