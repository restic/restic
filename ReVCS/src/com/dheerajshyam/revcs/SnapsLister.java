package com.dheerajshyam.revcs;

import java.io.*;
import java.security.*;

public class SnapsLister {
	
	public static void list_snaps() {
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
		
		File snapsFolder = new File("./.revcs/snaps/");
		File[] snaps = snapsFolder.listFiles();
		
		for(File snap : snaps) {
			
			if(snap != null && snap.exists() && snap.isDirectory()) {
				
				File[] children = snap.listFiles();
				boolean isConfigExists = false;
				
				for(File child : children) {
					
					if(child.getName().equals("CONFIG")) {
						
						CONFIG config = CONFIG.deserialize(child.getPath(), secureKey);
						
						System.out.print(config.get_snap_date_time() + ", " +
							config.get_snap_name() + " ");
						
						for(var entry : config.getObjectsMap().entrySet()) {
							var key = entry.getKey();
							var value = entry.getValue();
							
							if(value.equals(config.get_root_hash())) {
								System.out.println("(root folder name: " + key + ")");
								break;
							}
						}
						
						isConfigExists = true;
						
						break;
					}
				}
				
				if(!isConfigExists) {
					System.err.println("error, CONFIG file is missing in the snapshot '" + snap.getName() + "'");
					System.exit(-1);
				}
			}
		}
	}
}
