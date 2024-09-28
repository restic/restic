package com.dheerajshyam.revcs;

import java.io.*;

import java.security.*;

public class Status {
	
	private FileNode rootNode;
	
	public Status() {
		
		class StatusHandler {
			public static void handle(FileNode node) {
				if(node.file.isDirectory()) {
					for(FileNode child : node.children) {
						if(!child.isObselete) {
							if(child.file.isDirectory() && !(child.children.isEmpty()))
								handle(child);
							else System.out.println("\t" + child.file.getPath());
						} else System.out.println("\t" + child.file.getPath() + " (ignored)");
					
					}
				} else System.out.println("\t" + node.file.getPath());
			}
		}
		
		String secureKeyName = null;
		
		try {
			System.out.println("Please enter the name of the secure key that you used while adding files.");
			
			BufferedReader secureKeyNameReader = new BufferedReader(new InputStreamReader(System.in));
			secureKeyName = secureKeyNameReader.readLine();
			
			if(secureKeyName == null || secureKeyName.isBlank() || secureKeyName.isEmpty())
				throw new Exception();
			
		} catch(Exception e) {
			System.err.println("error, input stream failed.");
			System.exit(-1);
		}
		
		Key secureKey = EncryptionBuilder.generateKeyFromPath("./.revcs/" + secureKeyName);
		
		this.rootNode = FileNode.deserialize("./.revcs/INDEX", secureKey);
		
		if(this.rootNode != null || !this.rootNode.children.isEmpty()) {
			System.out.println("\nFiles currently added that are to be snapshotted:\n");
			StatusHandler.handle(rootNode);
			System.out.println();
		} else {
			System.out.println("error, index file size is less than expected.");
		}
	}
}
