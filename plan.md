## **Project Overview**

This project aims to build a **Bedrock Edition version of MCSR Ranked** using a **server-based architecture** running entirely on your personal computer. Up to **20 matches** can run simultaneously. Each 1v1 match places two players in **separate independent instances** (worlds) but with **identical seeds, terrain, structures, RNG inputs, and world settings** to ensure fairness and comparability.

All match instances are launched, monitored, and cleaned up by a central **Match Manager** running on a main controller process.

## **Architecture**

### **1. Main Controller**

- Runs on your computer.
  
- Starts and stops match servers.
  
- Ensures a maximum of **20 concurrent matches**.
  
- Syncs config and world generation parameters across both players’ worlds.
  
- Applies Mods & Data Packs automatically (including mods affecting RNG fairness).
  

### **2. Match Instances (One per Player)**

- Each player receives **their own separate world**, isolated to avoid interference.
  
- Worlds use the **same seed and worldgen parameters**, guaranteeing identical terrain and structure placement.
  
- RNG behavior is **fully synchronized** so that:
  
  - killing the same number of blazes yields the same number of blaze rods
    
  - trading with piglins yields the same number of pearls with the same timing and probabilities
    
  - killing endermen yields identical pearl-drop patterns
    
  - *all* loot tables (chests, mob drops, barters, fishing, etc.) behave identically for both players
    
- Configurable to use custom mods for deterministic RNG and structure generation.
  

### **3. Match Flow**

- Match Manager assigns both players to a match slot.
  
- Two child server instances launch with synced configs.
  
- Players spawn simultaneously and begin their speedrun race.
  
- Automatic win detection via:
  
  - Ender Dragon death
    
  - Forfeit command
    
  - Timeout / draw request
    
  - Mutual seed-change request
    

## **Commands & Player Interaction**

### **Forfeit Command (`/forfeit`)**

- Instantly ends the match. Opponent wins.

### **Draw Request (`/draw`)**

- Expires after **30 seconds** unless both players accept.

### **Seed-Change Request (`/seedchange`)**

- Expires after **30 seconds** unless both players accept.
  
- Regenerates both worlds with a new synchronized seed and RNG state.
  

## **Spectator Mode: Replay-Style**

- After each match, the system generates a **replay** of both runs.
  
- Similar to ReplayMod-style playback, but reconstructed from logged player actions.
  
- Avoids live performance impact.
  

## **Mods & RNG Enhancement**

You **do** want mods, so we plan for:

- deterministic RNG synchronization across both player worlds
  
- controlled blaze rod, pearl, enderman, and chest-loot outcomes
  
- structure RNG syncing
  
- faster optimized worldgen
  
- replay data extraction mods/add-ons
