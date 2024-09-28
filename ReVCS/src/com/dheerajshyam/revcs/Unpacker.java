package com.dheerajshyam.revcs;

import java.io.*;
import java.security.*;

import java.util.*;
import java.util.zip.*;

public class Unpacker {
	private CONFIG config;
	
	private boolean isDirectoryObject(String hash) {
		
		var objects_type_map = config.getObjectsTypeMap();
		
		if(!objects_type_map.containsKey(hash)) {
			System.err.println("Object '" + hash + "' not found.");
			System.exit(-1);
		}
		
		return objects_type_map.get(hash) == 1 ? true : false;
	}
	
	public Unpacker(String snapHash) {

		String secureKeyName = null;
		
		try {
			System.out.println("Please enter the name of the secure key that you have generated");
			
			BufferedReader secureKeyNameReader = new BufferedReader(new InputStreamReader(System.in));
			secureKeyName = secureKeyNameReader.readLine();
			
			if(secureKeyName == null || secureKeyName.isBlank() || secureKeyName.isEmpty())
				throw new Exception();
			
		} catch(Exception e) {
			System.err.println("error, input stream failed.");
			System.exit(-1);
		}
		
		Key key = EncryptionBuilder.generateKeyFromPath("./.revcs/" + secureKeyName);
		
		String configPath = "./.revcs/snaps/" + snapHash + "/CONFIG";
		
		File configFile = new File(configPath);
		if(configFile == null || !(configFile.exists())) {
			System.err.println("Snapshot or capsule not found to unpack.");
			System.exit(-1);
		}

		config = CONFIG.deserialize(configPath, key);
		if(config == null) {
			System.err.println("Unable to rebuild config from the provided path.");
			System.exit(-1);
		}
		
		String root_hash = config.get_root_hash();
		
		if(isDirectoryObject(root_hash)) {
			
			File rootDirectory = new File("./.revcs/snaps/" + snapHash + "/" + root_hash);
			if(!rootDirectory.exists()) {
				System.err.println("Object '" + rootDirectory.getPath() + "' missing in the snapshot.");
				System.exit(-1);
			}
			
			DirectoryObject rootDirectoryObject = DirectoryObject.deserialize(rootDirectory.getPath(), key);
			if(rootDirectoryObject != null) {
				
				File directory = new File("./" + rootDirectoryObject.dirName);
				
				boolean isDirectoryCreated = directory.mkdir();
				
				if(!isDirectoryCreated) {
					System.err.println("Unable to create root directory.");
					System.exit(-1);
				}
				
				class FilesIterator {
					Stack<String> parents = new Stack<String>();
					public int itemsCount = 0;
					
					
					public void iterate(DirectoryObject object, String snap_hash, Key key) {
						
						for(String file_name : object.fileBlocks) {
							
							String file_name_hash = HashBuilder.getHash(file_name);
							
							if(isDirectoryObject(file_name_hash)) {
								
								try {
									
									DirectoryObject directoryObject = DirectoryObject.deserialize("./.revcs/snaps/" +
										snap_hash + "/" + file_name_hash, key);
									
									String directoryPath = "";
									
									if(parents.isEmpty())
										directoryPath = rootDirectoryObject.dirName + "/" + directoryObject.dirName;
									else directoryPath = parents.peek() + "/" + directoryObject.dirName;
									
									System.out.println("Unpacking directory: " + directoryPath);
									
									File directory = new File(directoryPath);
									
									boolean isDirectoryCreated = directory.mkdir();
									if(!isDirectoryCreated) {
										System.err.println("Unable to create the directory '" + directory.getName() + "'");
										System.exit(-1);
									}
									
									itemsCount += 1;
									
									parents.push(directoryPath);

									iterate(directoryObject, snap_hash, key);
									
									parents.pop();
									
								} catch(Exception e) {
									
									System.err.println("internal error occurred.");
									System.exit(-1);
									
								}
								
							} else {
								
								try {
									
									FileObject fileObject = FileObject.deserialize("./.revcs/snaps/" +
										snap_hash + "/" + file_name_hash, key);
									
									String filePath = "";
									
									if(!parents.isEmpty())
										filePath = parents.peek() + "/" + fileObject.file_name;
									else filePath = rootDirectoryObject.dirName + "/" + fileObject.file_name;
									
									System.out.println("Unpacking file: " + filePath);
									
									File file = new File(filePath);
									
									boolean isFileCreated = file.createNewFile();
									if(!isFileCreated) {
										System.err.println("Unable to create the file '" + file.getName() + "'");
										System.exit(-1);
									}
									
									
									ByteArrayOutputStream byteArrayOutputStream  = new ByteArrayOutputStream();
									InflaterInputStream inflaterInputStream = new InflaterInputStream(
											new ByteArrayInputStream(fileObject.data));
									
									byte outputBytes[] = new byte[fileObject.file_size];
									
									int length = 0;
									
									while((length = inflaterInputStream.read(outputBytes)) != -1) {
										
										if(length == 0)
											break;
										
										byteArrayOutputStream.write(outputBytes, 0, length);
									}
									
									
									inflaterInputStream.close();
									
									byte[] decompressedBytes = byteArrayOutputStream.toByteArray();
									byteArrayOutputStream.close();
									
									FileOutputStream fileOutputStream = new FileOutputStream(file);
									fileOutputStream.write(decompressedBytes);
									fileOutputStream.close();
									
									itemsCount += 1;
									
									
									
								} catch(Exception e) {
									e.printStackTrace();
									System.err.println("internal error occurred.");
									System.exit(-1);
								}
							}
						}
					}
				}
				
				var files_iterator = new FilesIterator();
				files_iterator.iterate(rootDirectoryObject, snapHash, key);
				
				System.out.println("Unpacked snap successfully, total items unpacked: " + files_iterator.itemsCount);
			}
				
			else {
				System.err.println("internal error occurred.");
				System.exit(-1);
			}
		}
	}
}
