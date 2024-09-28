package com.dheerajshyam.revcs;

import java.nio.file.*;

import java.io.*;

import java.security.*;

import java.util.*;


final public class Staging {
	
	static {
		System.loadLibrary("native");
	}
	
	private FileNode fileRoot;
	
	private void buildFsTree(File providedFile) {
		
		Stack<FileNode> parentStack = new Stack<FileNode>();

		class FsTreeBuilder {
			
			public void build(File providedFile) {
				
				if(providedFile.isDirectory()) {
					
					for(File file : providedFile.listFiles()) {
						
						FileNode node = new FileNode();
						node.file = file;
						node.hash = HashBuilder.getHash(node.file.getPath());
						
						if(!parentStack.isEmpty())
							parentStack.peek().children.add(node);
						else fileRoot.children.add(node);
						
				        if(file.isDirectory()) {
				        	parentStack.push(node);
				        	this.build(file);
				        	parentStack.pop();
				        }
				    }
					
				} else {
					
					FileNode fs = new FileNode();
					fs.file = providedFile;
					fs.hash = HashBuilder.getHash(providedFile.getPath());
					
					if(!parentStack.isEmpty())
						parentStack.peek().children.add(fs);
					else fileRoot.children.add(fs);
				}
			}
		}
		
		new FsTreeBuilder().build(providedFile);
	}
	
	private void stage(File indexFile, Key secureKey) {
		var script = "";
		var mapping = this.fileRoot.nodeToDirMap();
		
		for(var entry : mapping.entrySet()) {
			var value = entry.getValue();
			
			for(var it = value.iterator(); it.hasNext();)
				script += "# ignore " + it.next() + "\n";
			
		}
		
		script += "\n# Commands:\n";
		
		var commands = new HashMap<String, String>();
		commands.put("# i (or) ignore <file_name>", "Ignore's a file that is not be added during staging.");
		
		for(var entry : commands.entrySet() ) {
			var key = entry.getKey();
			var value = entry.getValue();
			
			script += key + " = " + value + "\n";
		}
		
		File todo_file = new File("./revcs-add-todo");
		if(!todo_file.exists()) {
			try {
				if(!todo_file.createNewFile())
					throw new Exception("internal error, unable to generate script now.");
			} catch(Exception e) {
				System.err.println(e.getMessage());
				System.exit(-1);
			}
		}
		
		try {
			
			var file_writer = new FileOutputStream(todo_file);
			file_writer.write(script.getBytes());
			
			file_writer.close();
			
			this.open_editor(todo_file.getName());
			
			var file_reader = new FileInputStream(todo_file);
			
			script = new String(file_reader.readAllBytes());
			
			file_reader.close();
			
			todo_file.delete();
			
		} catch(Exception e) {
			
			System.err.println("Unable to perform staging at this moment.");
			System.exit(-1);
		}
		
		
		
		var lines = script.split("\n");
		var lineno = 1;
		
		for(var line : lines) {
			line = line.strip();
			if(!line.isEmpty() && !line.startsWith("#")) {
				var words = line.split(" ");
				if(words.length >= 2) { 
					var command = words[0];
					var accessIndex = 1;
					
					while(words[accessIndex].isBlank() || words[accessIndex].isEmpty()) {
						accessIndex++;
						if(accessIndex == words.length) {
							System.err.println("error, no file name provided to ignore.");
							System.exit(-1);
						}
					}
					
					var file_name = words[accessIndex];
					
					if(command.equals("ignore") || command.equals("i")) {
						
						File file = new File(file_name);
						
						if(!file.exists()) {
							System.err.println("error, ignoring an invalid file " + file_name);
							System.exit(-1);
						}
						
						if(!(fileRoot.removeChild(HashBuilder.getHash(file_name)))) {
							System.err.println("Unable to ignore the file '" + file_name + "', maybe deleted.");
							System.exit(-1);
						}
						
					} else {
						System.err.println("Invalid command '"+ command +"' provided in line "
							+ String.valueOf(lineno) + ".");
						System.exit(-1);
					}
				}
			}
			
			lineno++;
		}
		
		if(!(FileNode.serialize(fileRoot, indexFile, secureKey))) {
			
			System.err.println("internal error occurred, unable to perform adding now.");
			System.exit(-1);
			
		} else System.out.println("File(s) successfully added.");
		
	}
	
	private native void open_editor(String file_name);
	
	public Staging(String providedPath) {
			
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
		
		Key secureKey = EncryptionBuilder.generateKeyFromPath("./.revcs/" + secureKeyName);
		File indexFile = new File("./.revcs/INDEX");
		
		if(indexFile == null || !indexFile.exists()) {
			System.err.println("Index file does not exists.");
			System.exit(-1);
		}
		
		File providedFile = new File(Paths.get(providedPath).toAbsolutePath().toString());
		
		if(providedFile == null || !providedFile.exists()) {
			System.err.println("error, provided path '" + providedFile.getPath() + "' does not exists,"
				+ " unable to perform adding now.");
			
			System.exit(-1);
		}
		
		fileRoot = new FileNode();
		
		if(providedFile.isDirectory())
			fileRoot.file = providedFile;
		else fileRoot.file = providedFile.getParentFile();
		
		fileRoot.hash = HashBuilder.getHash(fileRoot.file.getName());
		
		this.buildFsTree(providedFile);
		
		if(fileRoot != null && !(fileRoot.children.isEmpty())) {
			this.stage(indexFile, secureKey);
		}
	}
}