package com.dheerajshyam.revcs;

import java.io.*;
import java.security.*;

final public class Initializer {
	
	private File initFolder, indexFile, snapsFolder;
	private Key secureKey;
	
	public Initializer() {
		
		initFolder = new File("./.revcs");
		
		if(!initFolder.exists()) {
			boolean isInitFolderCreated = initFolder.mkdir();
			if(!isInitFolderCreated) {
				System.err.println("internal error occurred, unable to initiate the capsule.");
				System.exit(-1);
			}
		}
		
		indexFile = new File(initFolder.getPath() + "/INDEX");
		
		if(!indexFile.exists()) {
			try {
				boolean isIndexFileCreated = indexFile.createNewFile();
				if(!isIndexFileCreated)
					throw new Exception();
			} catch(Exception e) {
				System.err.println("internal error occurred, unable to initiate the capsule.");
				System.exit(-1);
			}
		}
		
		snapsFolder = new File(initFolder.getPath() + "/snaps");
		
		if(!snapsFolder.exists()) {
			try {
				boolean isSnapsFolderCreated = snapsFolder.mkdir();
				if(!isSnapsFolderCreated)
					throw new Exception();
			} catch(Exception e) {
				System.err.println("internal error occurred, unable to initate the capsule.");
				System.exit(-1);
			}
		}
		
		this.secureKey = EncryptionBuilder.generateKey();
		
		if(this.secureKey == null) {
			System.err.println("internal error, unable to generate secure capsule key.");
			System.exit(-1);
		}
		
		System.out.println("Initialized an empty revcs capsule in the path '"
			+ initFolder.getParentFile().getPath() + "' successfully.");
	}
}
