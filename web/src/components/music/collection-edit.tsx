import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsString, useQueryState } from "nuqs";
import { z } from "zod";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAppForm } from "@/hooks/use-app-form";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";

export function CollectionEditButton({kind,source}:{kind:"album"|"artist";source:{id:string;name:string}}) {
 const [,setAlbum]=useQueryState("editAlbum",parseAsString);
 const [,setArtist]=useQueryState("renameArtist",parseAsString);
 return <Button size="sm" variant="outline" aria-label={`${kind==="album" ? "Edit album metadata" : "Rename artist"}: ${source.name}`} onClick={()=>{void (kind==="album" ? setAlbum : setArtist)(source.id);}}>{kind==="album" ? "Edit metadata" : "Rename artist"}</Button>;
}
export function ArtistRenameHost() {
 const [id,setID]=useQueryState("renameArtist",parseAsString.withDefault(""));
 return id ? <ArtistRename key={id} id={id} close={()=>void setID(null)} /> : null;
}
function ArtistRename({id,close}:{id:string;close:()=>void}) {
 const result=useQuery({queryKey:["api","/metadata/artists",id],queryFn:({signal})=>api<{name:string;songs:number}>(`/metadata/artists/${encodeURIComponent(id)}`,"GET",undefined,undefined,signal)});
 return <Dialog open onOpenChange={open=>{if(!open)close();}}><DialogContent className="sm:max-w-lg">
  <DialogTitle>Rename artist</DialogTitle>
  <DialogDescription>Update this artist’s track and album-artist credits across the shared library. Other artists and song titles are preserved.</DialogDescription>
  {result.isPending && <Text role="status">Loading artist…</Text>}
  {result.error && <Text role="alert">{result.error.message}</Text>}
  {result.data && <ArtistRenameForm id={id} name={result.data.name} songs={result.data.songs} close={close} />}
 </DialogContent></Dialog>;
}
function ArtistRenameForm({id,name,songs,close}:{id:string;name:string;songs:number;close:()=>void}) {
 const client=useQueryClient();
 const save=useMutation({mutationFn:(value:{name:string})=>api(`/metadata/artists/${encodeURIComponent(id)}`,"PUT",value),onSuccess:async()=>{toast.success(`Artist rename queued for ${songs} songs`);await client.invalidateQueries({queryKey:["api","/metadata/jobs"]});close();}});
 const form=useAppForm({defaultValues:{name},validators:{onSubmit:z.object({name:z.string().trim().min(1).max(500)})},onSubmit:async({value})=>{if(!save.isPending) await save.mutateAsync(value).catch(()=>undefined);}});
 return <form className="space-y-3" onSubmit={event=>{event.preventDefault();void form.handleSubmit();}}>
  <form.AppField name="name">{field=><field.TextField label="Artist name" disabled={save.isPending} />}</form.AppField>
  <Text className="m-0 text-xs">{songs} songs will be updated. File paths and audio stay unchanged.</Text>
  {save.error && <Text role="alert">{save.error.message}</Text>}
  <div className="flex justify-end gap-2"><Button type="button" variant="ghost" disabled={save.isPending} onClick={close}>Cancel</Button><Button disabled={save.isPending}>Save changes</Button></div>
 </form>;
}
