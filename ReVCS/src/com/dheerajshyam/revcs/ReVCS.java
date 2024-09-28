package com.dheerajshyam.revcs;

import java.io.*;
import java.util.*;


final public class ReVCS {
	
	public static void main(String[] args) {
		
		class ArgsHandler {
			
			private ArrayList<String> arguments;
			
			private Iterator<String> it;
			
			private LinkedHashMap<String, String> commands;
			
			public ArgsHandler() {
				arguments = new ArrayList<String>(
					Arrays.asList(args)
				);
				
				it = arguments.iterator();
				
				handle();
			}
			
			private void buildCommands() {
				if(commands == null)
					commands = new LinkedHashMap<String, String>();
				
				commands.put("--version", "Displays the current version of revcs.");
				commands.put("init", "Intiates a new empty capsule in the current working directory.");
				commands.put("add . or <file_name>", "Adds the current directory if provided . or particular file "
					+ "into staging phase and indexes it.");
				commands.put("store \"<snap_name>\" or <snap_name>", "Snapshots the staged contents and stores them "
					+ "into the capsule as snapshot as the <snap_name> provided.");
				commands.put("generate-key", "Generates the secret key.");
				commands.put("get-snap-hash <snap_name>", "Display's the snap object hash of "
					+ "the <snap_name> provided.");
				commands.put("status", "Shows the currently added files in the staging area.");
				commands.put("unpack <snap_name>", "Unpacks the objects in <snap_name> and rebuilds the codebase as"
					+ " it is on the disk.");
				commands.put("list", "Lists all the snapshots in ascending order based on date-wise in the capsule.");
				commands.put("build <snap_name>", "Builds the ReAr archive of the provided <snap_name> snapshot.");
			}
			
			private void verify_capsule() {
				if(!new File("./.revcs").exists()) {
					System.err.println("error, no valid revcs capsule found.");
					System.exit(-1);
				}
			}
			
			
			private void handle() {
				buildCommands();
				
				if(arguments.isEmpty())		
					this.handle_help();
				
				else {
					
					String argument = it.next();
					
					if(argument != null) {
						
						if(argument.equals("--version"))
							System.out.println("0.99");
						
						else if(argument.equals("init")) {
							if(!new File("./.revcs").exists())
								new Initializer();
							else {
								System.err.println("A revcs capsule already exists.");
								System.exit(-1);
							}
						}
						
						else if(argument.equals("add"))
							
							if(!it.hasNext())
								System.err.println("error, missing file-name (or) folder-name (or) . after add command.");
							else {
								this.verify_capsule();
								new Staging(it.next());
							}
						
						else if(argument.equals("store"))
							
							if(!it.hasNext())
								System.err.println("error, missing snap-name after store command.");
							else {
								this.verify_capsule();
								new Storing(it.next());
							}
						
						else if(argument.equals("generate-key")) {
							this.verify_capsule();
							EncryptionBuilder.generateKey();
						}
						
						else if(argument.equals("get-snap-hash"))
							if(!it.hasNext())
								System.err.println("error, missing snap-name after get-snap-hash command.");
							else {
								this.verify_capsule();
								System.out.println(HashBuilder.getHash(it.next()));
							}
						
						else if(argument.equals("status"))
							new Status();
						
						else if(argument.equals("unpack"))
							if(!it.hasNext())
								System.err.println("error, missing snap-name after unpack command.");
							else {
								this.verify_capsule();
								new Unpacker(HashBuilder.getHash(it.next()));
							}
						
						else if(argument.equals("list")) {
							this.verify_capsule();
							SnapsLister.list_snaps();
						}
						
						else if(argument.equals("build")) {
							if(!it.hasNext())
								System.err.println("error, missing snap-name after build command.");
							else {
								this.verify_capsule();
								new ReArBuilder().build_archive(it.next());
							}
							
						}
						
						else System.err.println("Invalid command provided.");
					
					} else {
						
						System.out.println("Invalid command provided.");
						this.handle_help();
					}
				}
			}
			
			private void handle_help() {
				
				System.out.println("A Venkata Subbu Dheeraj Shyam Polavarapu Production\n"
					+ "\033[0;1mrevcs 0.99 © 2024\033[0m\n");
				System.out.println("revcs commands guide\n");
				for(String command_name : commands.keySet())
					System.out.println("\t\033[0;1m" + command_name + "\033[0m: " + commands.get(command_name) + "\n");
			}
		}
		
		new ArgsHandler();
	}
}