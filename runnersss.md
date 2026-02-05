  tower.dev has two runner models:                                                                                                                   
                                                                                                                                                     
  1. Tower Cloud (managed) - Ephemeral, shared infrastructure. Tower provisions a runner on-demand, downloads your code, runs it, then wipes all     
  data. You never see or manage runners. They're drawn from a shared pool - not permanently allocated to your team.                                  
  2. Self-Hosted - Persistent runners on your own infrastructure, scoped to your account. You install them, they only serve your workloads.          
                                                                                                                                                     
  Your question is spot-on. In the Tower Cloud model, when you do tower run, a runner is dynamically allocated from Tower's pool, executes your work,
   and is cleaned up afterward. No runner sits idle waiting for just your team.

  What MiniTower currently implements is purely the self-hosted model - runners register to a team and permanently poll for that team's work only.
  This is a valid and correct model for what it is, but it doesn't reflect the full tower.dev picture.

  The real question is: does this matter for MiniTower's MVP?

  I'd argue no - and here's why:

  - MiniTower has no managed cloud infrastructure to provision from, so the "cloud runner" model doesn't apply
  - The self-hosted model is what PLAN.md explicitly targets (line 9: "self-hosted runner")
  - Even in tower.dev, self-hosted runners ARE scoped to accounts - your current design matches that exactly
  - The ephemeral shared-pool model would require container/VM orchestration (Kubernetes, etc.) which is explicitly deferred in PLAN.md (line 52:
  "Hardened sandboxing - containers/microVMs")

  If you wanted to move toward the shared-pool model in the future, the key change would be: instead of runners being bound to a team at
  registration, the control plane would manage a pool of runners and assign them to teams dynamically per-run, with full cleanup between runs.
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
 ☐ Direction 
                                              
Given this, what would you like to do?    

❯ 1. Keep as-is                                                                                                                                      
     The self-hosted model is correct for MVP. No changes needed.
  2. Add a note/backlog item                                                                                                                         
     Document the shared-pool runner model as a post-MVP evolution in PLAN.md                                                                      
  3. Redesign runners                
     Decouple runners from teams now, moving toward the dynamic allocation model

