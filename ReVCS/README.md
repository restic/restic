ReVCS is a simplified, nearly-mimicking version control system developed with an aim to make lives of individual or small group of developers easier.

# Source Guide

<b>native/</b> - JNI code

<b>src/com/dheerajshyam/revcs - Actual Java Source Code</b>
  1. Common_Classes.java - Classes commonly used by all other sources files.
  2. CryptoHandler.java - Hashing and Secret-Key-Generation related code.
  3. Initializer.java - Basic revcs capsule initializer code.
  4. ReVCS.java - Entry class file (arg-handler).
  5. SnapsLister.java - Listing of all the snapshots in the capsule related code.
  6. Staging.java - Second phase of snapshot creation process handling code.
  7. Status.java - Status command handler code.
  8. Storing.java - Third phase of snapshot creation process handling code.
  9. Unpacker.java - Revcs Snapshot to actual file system data converting related code.

<b>LICENSE</b> - GPL 3.0 License<br/>
<b>README.md </b> - This readme file<br/>
<b>guava-33.2.0-jre.jar </b> - Guava Java Jar Dependency<br/>

# Testing Guide
To test the working of ReVCS without the need of building it from source code, the only official way is to use the <b>official docker image</b>. <br/>

<b>Steps to follow</b>

1. Pull the docker image using following command: <b>docker pull dheerajshyampvsofficial/revcs:0.99A</b>

2. Enter into the root folder where you want to test the revcs using <b>cd root_folder_name</b>

3. <b>Run the following command</b> <br/><br/>
(<b>Windows</b>) docker run -it -v %cd%:/repo --rm dheerajshyampvsofficial/revcs:0.99A <br/>
(<b>Linux or macOS</b>) docker run -it -v $(pwd):/repo --rm dheerajshyampvsofficial/revcs:0.99A <br/>

4. Set an alias for the following command: "java -Djava.library.path=/revcs -jar /revcs/revcs.jar" as "revcs" to avoid using the long command everytime.

From here, follow this channel https://www.youtube.com/@DheerajShyamPVSOfficial to understand how to work with revcs.