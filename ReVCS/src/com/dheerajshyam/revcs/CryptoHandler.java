package com.dheerajshyam.revcs;

import java.io.*;

import java.nio.charset.*;

import java.security.*;
import java.security.spec.*;

import java.util.regex.*;

import javax.crypto.*;
import javax.crypto.spec.*;

import com.google.common.hash.*;

class HashBuilder {
	
	public static String getHash(String info) {
		
		String hash = Hashing.sha384()
			.hashString(info, StandardCharsets.UTF_8)
			.toString();
		
		return hash;
	}
	
	public static String getHashFromBytes(byte[] bytes) {
		Hasher hasher = Hashing.sha384().newHasher();
		hasher.putBytes(bytes);
		
		return hasher.hash().toString();
	}
}

final class EncryptionBuilder {
	
	protected static SecretKey _generateKey() {
		SecretKey secureKey = null;
		
		try {
			
			BufferedReader nameReader = new BufferedReader(new InputStreamReader(System.in));
			System.out.println("Please provide a name for secret key");
			
			String secureKeyName = nameReader.readLine();
			if(secureKeyName.isEmpty() || secureKeyName.isBlank() || secureKeyName == null)
				throw new Exception();
			
			secureKeyName = "./.revcs/" + secureKeyName;
			
			BufferedReader passwordReader = new BufferedReader(new InputStreamReader(System.in));
			System.out.println("Please provide a password to generate secret key.");
			
			String secureKeyPassword = passwordReader.readLine();
			if(secureKeyPassword.isEmpty() || secureKeyPassword.isBlank()
					|| secureKeyPassword == null) {
				System.err.println("error, password should not be empty or contain "
					+ "blank spaces.");
				System.exit(-1);
			}
			
			if(secureKeyPassword.length() <= 12) {
				System.err.println("error, password must be atleast 12 characters long");
				System.exit(-1);
			} else {
				String validPasswordRegex = "^(?=.*[a-z])(?=."
                   + "*[A-Z])(?=.*\\d)"
                   + "(?=.*[-+_!@#$%^&*., ?]).+$";
				Pattern pattern = Pattern.compile(validPasswordRegex);
				
				Matcher matcher = pattern.matcher(secureKeyPassword);
				
				if(!matcher.matches()) {
					System.err.println("error, password must contain uppercase, lowercase"
							+ ", numbers and if possible special characters.");
					System.exit(-1);
				}
			}
			
			SecretKeyFactory factory = SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256");
			KeySpec spec = new PBEKeySpec(secureKeyPassword.toCharArray(), "=tVEp/v?xYM)".getBytes(), 65536, 256);
		    secureKey = new SecretKeySpec(factory.generateSecret(spec).getEncoded(), "AES");
			
			try(ObjectOutputStream oos = new ObjectOutputStream(
				new FileOutputStream(secureKeyName))) {
				
				oos.writeObject(secureKey);
				
				System.out.println("Created key and stored in file: " + secureKeyName);
			}
			
		} catch(Exception e) {
			System.err.println("Unable to generate key.");
			System.exit(-1);
		}
		
		
		if(secureKey == null) {
			System.err.println("Unable to generate key.");
			System.exit(-1);
		}
		
		return secureKey;
	}
	
	protected static Key _generateKeyFromPath(String keyPath) {
		Key secureKey = null;
		
		try(ObjectInputStream ois = new ObjectInputStream(
			new FileInputStream(keyPath))) {
			
			secureKey = (Key) ois.readObject();
			
		} catch(Exception ignored) {}
		
		if(secureKey == null) {
			System.err.println("Unable to generate key from path '" + keyPath + "'.");
			System.exit(-1);
		}
		
		return secureKey;
	}
	
	public static Key generateKeyFromPath(String keyPath) {
		return _generateKeyFromPath(keyPath);
	}
	
	public static Key generateKey() {
		return _generateKey();
	}
}
