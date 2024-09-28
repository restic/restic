package com.dheerajshyam.revcs;

import java.io.*;

import java.security.*;

import java.util.*;

import javax.crypto.*;

final class FileNode implements Serializable {
	
	private static final long serialVersionUID = 2235661293297664965L;
	
	public File file;
	public String hash;
	public ArrayList<FileNode> children;
	public boolean isObselete = false;
	
	public FileNode() {
		children = new ArrayList<FileNode>();
	}
	
	public boolean removeChild(String hash) {
		
		class ChildRemover {
			
			public boolean isRemoved = false;
			
			public void makeObselete(FileNode node, String hash) {
				
				if(node.hash.equals(hash)) {
					node.isObselete = isRemoved = true;
					System.out.println("Ignoring the file " + node.file.getPath() + "\n");
				}
				else {
					
					if(!node.children.isEmpty())
						for(FileNode child : node.children)
							makeObselete(child, hash);
				}
			}
		}
		
		ChildRemover childRemover = new ChildRemover();
		childRemover.makeObselete(this, hash);
		
		return childRemover.isRemoved;
	}
	
	public HashMap<String, ArrayList<File>> nodeToDirMap() {
		
		HashMap<String, ArrayList<File>> dirMap = new HashMap<String, ArrayList<File>>();
		
		class DirMapBuilder {
			
			public void build(FileNode node) {
				
				ArrayList<File> children = new ArrayList<File>();
				
				if(node.file.isDirectory() && !node.children.isEmpty()) {
					
					for(FileNode child : node.children) {
						
						if(!child.children.isEmpty()) {
							build(child);
						} else children.add(child.file);
					}
					
					dirMap.put(node.file.getPath(), children);
					
				} else  {
					children.add(node.file);
					dirMap.put(node.file.getParentFile().getPath(), children);
				}
			}
		}
		
		DirMapBuilder dirMapBuilder = new DirMapBuilder();
		dirMapBuilder.build(this);
		
		return dirMap;
	}
	
	public static boolean serialize(FileNode node,
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
			System.err.println("internal error occurred, exiting with status -1.");
			System.exit(-1);
		}
		
		return isStored;
	}
	
	public static FileNode deserialize(String filePath, Key key) {
		
		FileNode node = null;
		
		if(key != null) {
			try (ObjectInputStream ois = new ObjectInputStream(
				new FileInputStream(filePath))) {
				
				SealedObject sealedNode = (SealedObject) ois.readObject();
	            node = (FileNode) sealedNode.getObject(key);
	            
	        } catch (Exception e) {
	        	System.err.println("internal error occurred, exiting with status -1.");
				System.exit(-1);
	        }
		}
		
		return node;
	}
}