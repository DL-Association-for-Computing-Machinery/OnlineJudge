#!/bin/sh
set -eu
if [ "${1-}" = "-dumpfullversion" ]; then
  exec /usr/local/oj-jdk/bin/javac -version
fi
# Fixed paths only. Annotation processing is unavailable to contestant code.
mkdir classes
/usr/local/oj-jdk/bin/javac -J-XX:ActiveProcessorCount=1 -J-XX:+UseSerialGC -J-Xmx192m -J-XX:-UsePerfData -proc:none --release 21 -encoding UTF-8 -d classes Main.java
# Stream directly to avoid the jar tool's temporary archive move.
exec /usr/local/oj-jdk/bin/jar -J-XX:ActiveProcessorCount=1 -J-XX:+UseSerialGC -J-Xmx64m -J-XX:-UsePerfData --create -C classes . > main.jar
