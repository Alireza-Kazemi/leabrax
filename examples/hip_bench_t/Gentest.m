nRow =30;
nCol = 5;
x = round(rand(nRow,nCol));
dm = squareform(pdist(x'));
save('dm.mat','dm','nRow','nCol')


nVar = nRow*nCol;
t=1;
F = @myCor;
pop1 = round(rand(10,nVar));
Opts = optimoptions('ga','InitialPopulationRange',[zeros(1,nVar);ones(1,nVar)],'InitialPopulationMatrix',pop1);
[B,fval] = ga(@myCor,nVar,ones(1,nVar),nVar,[],[],zeros(1,nVar),ones(1,nVar),[],Opts);
xPrime = round(reshape(B,nRow,nCol));
dmPrime = squareform(pdist(reshape(round(B),nRow,[])'));

function f = myCor(x)
    if(size(x,1)==1)
        x=x';
    end
%     dm  = [0,3.46410161513775,4,3.87298334620742,4.12310562561766;3.46410161513775,0,4,4.35889894354067,4.79583152331272;4,4,0,4.12310562561766,3.87298334620742;3.87298334620742,4.35889894354067,4.12310562561766,0,3.74165738677394;4.12310562561766,4.79583152331272,3.87298334620742,3.74165738677394,0];
    load dm.mat dm nRow
    f = (squareform(pdist(reshape(round(x),nRow,[])')) - dm).^2;
    f = mean(reshape(f,1,[]));
end





