import { test, expect } from "@playwright/test";

test("completed deletion restores only its file and removes the Restore action after success", async ({ page }) => {
  const tasks=[
    {id:"deleted",title:"Deleted song",kind:"other",state:"completed",stage:"Removed; recovery copy retained",canRestore:true},
    {id:"expired",title:"Expired song",kind:"other",state:"completed",stage:"Recovery copy expired",canRestore:false},
  ];
  const writes:string[]=[];
  let fail=true;
  await page.route("**/api/v2/**",async route=>{
    const path=new URL(route.request().url()).pathname;
    if(path.endsWith("/setup")) return route.fulfill({json:{required:false}});
    if(path.endsWith("/session")) return route.fulfill({json:{csrf:"fixture",user:{id:"admin",username:"admin",role:"admin",quality:["128"]}}});
    if(path==="/api/v2/activity") return route.fulfill({json:tasks});
    if(path==="/api/v2/tasks/deleted/restore") {
      writes.push(path);
      if(fail) return route.fulfill({status:409,json:{error:"Original path occupied; no file overwritten"}});
      tasks[0]!.canRestore=false;tasks[0]!.stage="Deleted file restored; playlist replacements unchanged";
      return route.fulfill({json:{restored:true}});
    }
    throw new Error("Unexpected API: "+path);
  });
  await page.goto("/activity");
  await page.getByRole("button",{name:/^Completed/}).click();
  const deleted=page.locator('[data-slot="activity-row"]').filter({hasText:"Deleted song"});
  const expired=page.locator('[data-slot="activity-row"]').filter({hasText:"Expired song"});
  await expect(expired.getByRole("button",{name:"Restore file",exact:true})).toHaveCount(0);
  await deleted.getByRole("button",{name:"Restore file",exact:true}).click();
  await expect(page.getByRole("alert")).toContainText("Original path occupied");
  fail=false;
  await deleted.getByRole("button",{name:"Restore file",exact:true}).click();
  await expect(deleted.getByRole("button",{name:"Restore file",exact:true})).toHaveCount(0);
  await expect(deleted).toContainText("playlist replacements unchanged");
  expect(writes).toEqual(["/api/v2/tasks/deleted/restore","/api/v2/tasks/deleted/restore"]);
});
