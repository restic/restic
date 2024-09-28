package com.dheerajshyam.revcs;

import java.io.*;
import java.security.*;
import java.util.*;

final class DataBlock implements Serializable {
	
	private static final long serialVersionUID = 7643910167062299112L;
	
	public String file_name;
	public byte contents[];
	int file_size;
	
	public DataBlock(String file_name, byte contents[], int file_size) {
		this.file_name = file_name;
		this.contents = contents;
		this.file_size = file_size;
	}
}

final class ReAr implements Serializable {
	
	private static final long serialVersionUID = 935592920555074713L;
	
	public Key secure_key;
	public String snap_name;
	public ArrayList<DataBlock> blocks; 
}

public class ReArBuilder {
	public void build_archive(String snap_name) {
		
		String secureKeyName = null;
		
		try {
			System.out.println("Please enter the name of the secure key that you have generated");
			
			BufferedReader secureKeyNameReader = new BufferedReader(new InputStreamReader(System.in));
			secureKeyName = secureKeyNameReader.readLine();
			
			if(secureKeyName == null || secureKeyName.isBlank() || secureKeyName.isEmpty())
				throw new Exception("error, secure-key name is blank.");
			
		} catch(Exception e) {
			System.err.println(e.getMessage());
			System.exit(-1);
		}
		
		Key key = EncryptionBuilder.generateKeyFromPath("./.revcs/" + secureKeyName);
		
		try {
			var snap_hash = HashBuilder.getHash(snap_name);
			
			File snapFolder = new File("./.revcs/snaps/" + snap_hash);
			if(snapFolder == null || !snapFolder.exists())
				throw new Exception("error, folder '" + snap_name + "' not found, might be deleted.");
			
			ReAr reAr = new ReAr();
			reAr.secure_key = key;
			reAr.snap_name = snap_hash;
			
			File[] snap_files = snapFolder.listFiles();
			
			for(File file : snap_files) {
				FileInputStream fileInputStream = new FileInputStream(file);
				
				byte[] fileBytes = fileInputStream.readAllBytes();
				
				fileInputStream.close();
				
				reAr.blocks.add(new DataBlock(file.getName(), fileBytes, fileBytes.length));
			}
					
		} catch(Exception e) {
			System.err.println(e.getMessage());
			System.exit(-1);
		}
	}
}
