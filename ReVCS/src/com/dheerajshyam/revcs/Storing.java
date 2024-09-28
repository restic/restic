package com.dheerajshyam.revcs;

import java.io.*;

import java.security.*;

import java.time.*;
import java.time.format.*;

import java.util.*;
import java.util.zip.*;

import javax.crypto.*;

import org.json.*;

import org.rocksdb.*;


final class FileObject implements Serializable {

	private static final long serialVersionUID = 1896702035169223705L;
	
	public String file_path, file_name, hash;
	public int file_size, compressed_size;
	public byte data[];
	
	public FileObject(String file_path, String file_name, int file_size,
		int compressed_size, byte data[]) {
		
		this.file_path = file_path;
		this.file_name = file_name;
		this.file_size = file_size;
		this.compressed_size = compressed_size;
		this.data = data;
		this.hash = HashBuilder.getHashFromBytes(data);
	}
	
	public static boolean serialize(FileObject node,
		File outputFile, Key secureKey) {
		
		boolean isStored = false;
		
		try {
			
			Cipher cipher = Cipher.getInstance("AES");
			cipher.init(Cipher.ENCRYPT_MODE, secureKey);
			
			SealedObject sealedNode = new SealedObject(node, cipher);
			
			try(ObjectOutputStream oos = new ObjectOutputStream(
				new FileOutputStream(outputFile.getAbsolutePath()))) {
				
				oos.writeObject(sealedNode);
				
				isStored = true;
				
			} catch(Exception e) {
				e.printStackTrace();
			}
			
		} catch(Exception e) {
			e.printStackTrace();
		}
		
		return isStored;
	}
	
	public static FileObject deserialize(String filePath, Key key) {
		
		FileObject node = null;
		
		if(key != null) {
			try (ObjectInputStream ois = new ObjectInputStream(
				new FileInputStream(filePath))) {
				
				SealedObject sealedNode = (SealedObject) ois.readObject();
	            node = (FileObject) sealedNode.getObject(key);
	            
	        } catch (Exception e) {
	        	e.printStackTrace();
	        }
		}
		
		return node;
	}
}


final class DirectoryObject implements Serializable {
	
	private static final long serialVersionUID = 8992224522097387703L;
	
	public String dirPath;
	public String dirName;
	public String hash;
	
	public ArrayList<String> fileBlocks;
	
	public DirectoryObject(String dirPath, String dirName) {
		this.dirPath = dirPath;
		this.dirName = dirName;
		this.fileBlocks = new ArrayList<String>();
	}
	
	public static boolean serialize(DirectoryObject node,
		File outputFile, Key secureKey) {
		
		boolean isStored = false;
		
		try {
			
			Cipher cipher = Cipher.getInstance("AES");
			cipher.init(Cipher.ENCRYPT_MODE, secureKey);
			
			SealedObject sealedNode = new SealedObject(node, cipher);
			
			try(ObjectOutputStream oos = new ObjectOutputStream(
				new FileOutputStream(outputFile.getAbsolutePath()))) {
				
				oos.writeObject(sealedNode);
				
				isStored = true;
				
			} catch(Exception e) {
				e.printStackTrace();
			}
			
		} catch(Exception e) {
			e.printStackTrace();
		}
		
		return isStored;
	}
		
	public static DirectoryObject deserialize(String filePath, Key key) {
		
		DirectoryObject node = null;
		
		if(key != null) {
			try (ObjectInputStream ois = new ObjectInputStream(
				new FileInputStream(filePath))) {
				
				SealedObject sealedNode = (SealedObject) ois.readObject();
	            node = (DirectoryObject) sealedNode.getObject(key);
	            
	        } catch (Exception e) {
	        	System.out.println(filePath);
	        	e.printStackTrace();
	        }
		}
		
		return node;
	}
}


final class CONFIG implements Serializable {

	private static final long serialVersionUID = 1943449171479437075L;
	
	private String root_hash, snap_name, datetime;
	private HashMap<String, String> objectsMap;
	private HashMap<String, Integer> objectsTypeMap;
	
	public CONFIG(String root_hash, String snap_name) {
		this.root_hash = root_hash;
		this.snap_name = snap_name;
		this.objectsMap = new HashMap<String, String>();
		this.objectsTypeMap = new HashMap<String, Integer>();
		
		LocalDateTime localTime = LocalDateTime.now();
		DateTimeFormatter dateTimeFormatter = DateTimeFormatter.ofPattern("dd-MM-yyyy HH:mm a");
		
		this.datetime = localTime.format(dateTimeFormatter);
	}
	
	public HashMap<String, String> getObjectsMap() {
		return this.objectsMap;
	}
	
	public HashMap<String, Integer> getObjectsTypeMap() {
		return this.objectsTypeMap;
	}
	
	public String get_snap_name() {
		if(snap_name == null || snap_name.isBlank() || snap_name.isEmpty()) {
			System.err.println("Unable to get snap name.");
			System.exit(-1);
		}
		
		return this.snap_name;
	}
	
	public String get_snap_date_time() {
		if(datetime == null || datetime.isBlank() || datetime.isEmpty()) {
			System.err.println("Unable to get date & time of the snap.");
			System.exit(-1);
		}
		
		return this.datetime;
	}
	
	public String get_root_hash() {
		if(root_hash == null || root_hash.isBlank() || root_hash.isEmpty()) {
			System.err.println("Unable to get root hash.");
			System.exit(-1);
		}
		
		return this.root_hash;
	}
	
	public void add_object_type(String hash, int type) {
		try {
			
			boolean isAdded = false;
			
			if(!hash.isBlank() || !hash.isEmpty() || hash != null) {
				this.objectsTypeMap.put(hash, type);
				isAdded = true;
			}
			
			if(!isAdded)
				throw new Exception();
			
		} catch(Exception e) {
			System.out.println("Unable to add object type.");
			System.exit(-1);
		}
	}
	
	public void add_object_hash(String name, String hash) {
		
		try {
			
			boolean isAdded = false;
			
			if(!name.isBlank() || !name.isEmpty() || name != null) {
				if(!hash.isBlank() || !hash.isEmpty() || hash != null) {
					this.objectsMap.put(name, hash);
					isAdded = true;
				}
			}
			
			if(!isAdded)
				throw new Exception();
			
		} catch(Exception e) {
			System.err.println("Unable to add object hash.");
			System.exit(-1);
		}
		
	}
	
	public String search_object(String name) {
		
		if(name.isEmpty() || name.isBlank() || name == null) {
			System.err.println("Search object name issue.");
			System.exit(-1);
		}
		
		String hash = this.objectsMap.get(name);
		if(hash == null || hash.isEmpty() || hash.isBlank()) {
			System.err.println("Search object hash issue.");
			System.exit(-1);
		}
		
		return hash;
	}
	
	public static boolean serialize(CONFIG config,
		File outputFile, Key secureKey) {
		
		boolean isStored = false;
		
		try {
			
			Cipher cipher = Cipher.getInstance("AES");
			cipher.init(Cipher.ENCRYPT_MODE, secureKey);
			
			SealedObject sealedNode = new SealedObject(config, cipher);
			
			try(ObjectOutputStream oos = new ObjectOutputStream(
				new FileOutputStream(outputFile.getAbsolutePath()))) {
				
				oos.writeObject(sealedNode);
				
				isStored = true;
				
			} catch(Exception e) {
				e.printStackTrace();
			}
			
		} catch(Exception e) {
			e.printStackTrace();
		}
		
		return isStored;
	}
		
	public static CONFIG deserialize(String filePath, Key key) {
		
		CONFIG node = null;
		
		if(key != null) {
			try (ObjectInputStream ois = new ObjectInputStream(
				new FileInputStream(filePath))) {
				
				SealedObject sealedNode = (SealedObject) ois.readObject();
	            node = (CONFIG) sealedNode.getObject(key);
	            
	        } catch (Exception e) {
	        	e.printStackTrace();
	        }
		}
		
		return node;
	}
}

final public class Storing {
	
	static {
		RocksDB.loadLibrary();
	}
	
	private FileNode rootNode;
	
	public void store(String snap_name, File snapFolder, Key secureKey, RocksDB db) {
		
		if(rootNode.children.isEmpty())
			System.out.println("Nothing to store, capsule is upto date.");
		else {
			
			String totalTime = "0";
			
			ArrayList<DirectoryObject> directoryObjects = new ArrayList<DirectoryObject>();
			ArrayList<FileObject> fileObjects = new ArrayList<FileObject>();
			
			CONFIG config = new CONFIG(HashBuilder.getHash(rootNode.file.getAbsolutePath()), snap_name);
			
			class ObjectsBuilder {
				
				public String totalTime = "0";
				
				private FileObject build_file_object(File file) {
					
					FileObject data_block = null;
					
					try {
						
						byte[] fileContentBytes = new byte[(int) file.length()];
						
						FileInputStream fileInputStream = new FileInputStream(file);
			            fileInputStream.read(fileContentBytes);
			            fileInputStream.close();
			            
			            String dbKey = HashBuilder.getHash(file.getName());
			            String dbValue = HashBuilder.getHash(new String(fileContentBytes));
			            
			            boolean canCalculateTime = true;
			            
			            if(db.keyExists(dbKey.getBytes())) {
			            	String prevDBValue = new String(db.get(dbKey.getBytes()));
			            	if(prevDBValue.equals(dbValue)) {
			            		System.out.print("No change in the file: " + file.getName() + ", ");
			            		System.out.println("no coins generated for this file.");
			            		canCalculateTime = false;
			            	}
			            }
			            
			            long start_time = System.currentTimeMillis();
						
						int fileContentSize = fileContentBytes.length;
						
						ByteArrayOutputStream byteArrayOutputStream = new ByteArrayOutputStream();
						
						DeflaterOutputStream deflaterOutputStream = new DeflaterOutputStream(byteArrayOutputStream);
						deflaterOutputStream.write(fileContentBytes);
						deflaterOutputStream.close();
						
						byte[] compressedBytes = byteArrayOutputStream.toByteArray();
						
						data_block = new FileObject(file.getAbsolutePath(), file.getName(), fileContentSize,
							compressedBytes.length, compressedBytes);
						
						byteArrayOutputStream.close();
						
						long end_time = System.currentTimeMillis() - start_time;
						
						if(canCalculateTime)
			            	totalTime += String.valueOf(end_time);
			            
			            db.put(
			            	dbKey.getBytes(),
			            	dbValue.getBytes()
	            		);
						
						
					} catch(Exception e) {
						
						e.printStackTrace();
						System.err.println("Object building caused an issue.");
						System.exit(-1);
					}
					
					return data_block;
				}
				
				public void build_objects(FileNode node) {
					
					if(node != null) {
						
						if(!node.file.exists()) {
							System.err.println("error, path '" + node.file.getPath() + "' not found.");
							System.exit(-1);
						}
						
						if(node.file.isDirectory()) {
						
							String nodePath = node.file.getAbsolutePath();
							String nodeFileName = node.file.getName();
							
							DirectoryObject directoryObject = new DirectoryObject(nodePath, nodeFileName);
							
							config.add_object_hash(nodePath, HashBuilder.getHash(nodePath));
							config.add_object_type(HashBuilder.getHash(nodePath), 1);
							
							for(FileNode child : node.children) {
								
								if(!child.file.isDirectory()) {
									
									if(!child.isObselete) {
										
										String childPath = child.file.getAbsolutePath();
										config.add_object_hash(childPath, HashBuilder.getHash(childPath));
										config.add_object_type(HashBuilder.getHash(childPath), 0);
										
										directoryObject.fileBlocks.add(childPath);
										fileObjects.add(this.build_file_object(child.file));
									}
									
								} else {
									
									directoryObject.fileBlocks.add(child.file.getAbsolutePath());
									build_objects(child);
								}
							}
							
							directoryObjects.add(directoryObject);
						}
					}
				}
			}
			
			ObjectsBuilder objectsBuilder = new ObjectsBuilder();
			objectsBuilder.build_objects(rootNode);
			
			totalTime = objectsBuilder.totalTime;
			
			if(directoryObjects.isEmpty() && fileObjects.isEmpty()) {
				System.out.println("Nothing to store, capsule upto date.");
				System.exit(0);
			}
			
			boolean isConfigSerialized = CONFIG.serialize(
				config,
				new File(snapFolder.getPath() + "/CONFIG"),
				secureKey
			);
			
			
			if(!isConfigSerialized) {
				System.err.println("Config unable to seralize.");
				System.exit(-1);
			}
			
			for(DirectoryObject directoryObject : directoryObjects) {
				
				File directoryObjectFile = new File(snapFolder.getPath() + "/"
					+ HashBuilder.getHash(directoryObject.dirPath));
				
				System.out.println("Processing the folder '" + directoryObject.dirName + "'");
				
				if(!directoryObjectFile.exists()) {
					try {
						boolean isCreated = directoryObjectFile.createNewFile();
						if(!isCreated)
							throw new Exception();
						
						boolean isDirectoryObjectSerialized = DirectoryObject.serialize(
							directoryObject,
							directoryObjectFile,
							secureKey
						);
						
						if(!isDirectoryObjectSerialized)
							throw new Exception();
						
					} catch(Exception e) {
						e.printStackTrace();
						
						System.err.println("Directory object issue.");
						System.exit(-1);
					}	
				}
			}
			
			for(FileObject fileObject : fileObjects) {
				File fileObjectFile = new File(snapFolder.getPath() + "/"
					+ HashBuilder.getHash(fileObject.file_path));
				
				System.out.println("Processing the file '" + fileObject.file_path + "'");
				
				if(!fileObjectFile.exists()) {
					try {
						boolean isCreated = fileObjectFile.createNewFile();
						if(!isCreated)
							throw new Exception();
						
						boolean isFileObjectSerialized = FileObject.serialize(
							fileObject,
							fileObjectFile,
							secureKey
						);
						
						if(!isFileObjectSerialized)
							throw new Exception();
						
					} catch(Exception e) {
						System.err.println("File object issue.");
						System.exit(-1);
					}	
				}
			}
			
			String balanceKey = HashBuilder.getHash("balancer");

			double balance = 0;
			
			try {
				
				if(db.keyExists(balanceKey.getBytes())) {
					balance = Double.parseDouble(
						new String(db.get(balanceKey.getBytes()))
					);
				}
				
				System.out.println("Old balance: " + balance);
				
				balance += ((Double.parseDouble(totalTime))/1000 * 0.01);
				
				db.put(balanceKey.getBytes(), String.valueOf(balance).getBytes());
				
				System.out.println("New balance: " + new String(db.get(balanceKey.getBytes())));

			} catch(Exception e) {}
			
			System.out.println("Snapshot stored successfully.");
			
		}
	}
	
	public Storing(String snap_name) {
		
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
		
		String snapPath = "./.revcs/snaps/" + HashBuilder.getHash(snap_name);
		
		File snapFolder = new File(snapPath);
		if(!snapFolder.exists()) {
			boolean isSnapFolderCreated = snapFolder.mkdir();
			if(!isSnapFolderCreated) {
				System.err.println("Snap folder not created.");
				System.exit(-1);
			}
		}
		
		
		String indexPath = "./.revcs/INDEX";
		
		try {
			
			File indexFile = new File(indexPath);
			
			if(indexFile == null || !indexFile.exists()) {
				System.err.println("Index file does not exists.");
				System.exit(-1);
			}
			
			if(indexFile.length() == 0) {
				System.out.println("Nothing to store, capsule is upto date.");
			} else {
				this.rootNode = FileNode.deserialize("./.revcs/INDEX", secureKey);
				if(this.rootNode != null) {
					
					Options options = new Options();
			        options.setCreateIfMissing(true);

			        try (RocksDB db = RocksDB.open(options, "./transactions")) {
			        	JSONObject details = new JSONObject();
						this.store(snap_name, snapFolder, secureKey, db);
						
			            db.close();

			        } catch (RocksDBException e) {
			        	e.printStackTrace();
			            for(File file : snapFolder.listFiles()) {
			            	if(!file.isDirectory())
			            		file.delete();
			            }
			            snapFolder.delete();
			            System.err.println("Snapshot creation failed.");
			        }
			        
			        options.close();
					
				} else {
					System.err.println("Root node is null.");
					System.exit(-1);
				}
			}
			
		} catch(Exception e) {
			e.printStackTrace();
		}
	}
	
}