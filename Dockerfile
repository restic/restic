# this dockerfile is inspired by
# https://registry.hub.docker.com/u/tomdesinto/docker-travis-run/dockerfile/
FROM ubuntu:precise

# install dependencies
RUN apt-get update
RUN apt-get install -y --no-install-recommends build-essential ruby1.9.1 ruby1.9.1-dev rubygems git libhighline-ruby1.9.1 libssl-dev
RUN update-alternatives --set ruby /usr/bin/ruby1.9.1
RUN update-alternatives --set gem /usr/bin/gem1.9.1

# install travis-cli
RUN gem install travis bundler coder activesupport --no-rdoc --no-ri

# install travis-build
RUN git clone https://github.com/travis-ci/travis-build /opt/travis-build

# install other dependencies
RUN git clone https://github.com/meatballhat/gimme /opt/gimme && ln -s /opt/gimme/gimme /usr/bin/gimme

# add and configure user
ENV HOME /home/travis
RUN useradd -m -d $HOME -s /bin/bash travis

# run everything below as user travis
USER travis
WORKDIR $HOME

RUN mkdir .travis
RUN ln -s /opt/travis-build .travis/travis-build

# RUN travis version

# ENTRYPOINT ["travis", "run", "--skip-version-check", "--skip-completion-check"]
# CMD ["-p"]

